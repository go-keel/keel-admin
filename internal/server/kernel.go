package server

import (
	"time"

	"github.com/go-keel/keel"
	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/audit"
	coredata "github.com/go-keel/keel-admin/internal/core/data"
	amw "github.com/go-keel/keel-admin/internal/core/middleware"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/sys/data"
	"github.com/go-keel/keel-admin/internal/core/sys/service"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel/core/kconfig"
	"github.com/go-keel/keel/core/kdb"
	"github.com/go-keel/keel/core/kredis"
	"github.com/go-keel/keel/middleware/sentinel"
	"github.com/go-keel/keel/mw"
	"github.com/go-keel/keel/server/api"
)

// Kernel holds the admin kernel's objects, built by NewKernel. The exported fields are what the business area may
// take from the kernel in di.go; the rest stays inside, so upgrading the kernel never changes di.go's wiring of it.
type Kernel struct {
	// RefRepo turns user and department IDs into display names (refs on list rows).
	RefRepo biz.RefRepo
	// ScopeUseCase implements perm.ScopeResolver (data scope) and perm.GrantRevoker (clearing explicit grants after a
	// business instance is deleted).
	ScopeUseCase *biz.ScopeUseCase
	// BusinessZone is sys.business_time_zone, loaded.
	BusinessZone *time.Location
	// Brand is sys.brand.
	Brand *conf.Brand

	config *kconfig.Config
	authUC *biz.AuthUseCase
	otpUC  *biz.OTPUseCase
	audit  *biz.AuditUseCase

	auth *service.AuthService
	dept *service.DeptService
	role *service.RoleService
	user *service.UserService
	meta *service.MetaService
	logs *service.AuditService
}

// NewKernel builds the kernel from the infrastructure di.go owns: the config, the sys node of app.yaml (read once at
// startup, so a change needs a restart), the admin database and Redis. The cleanup closes what the kernel opened
// itself (the captcha provider); call it even when err is not nil.
func NewKernel(config *kconfig.Config, sysConf *conf.Sys, adminDB *kdb.DB, redis *kredis.Redis) (*Kernel, func(), error) {
	businessZone, err := provideBusinessZone(sysConf)
	if err != nil {
		return nil, func() {}, err
	}
	// nil when sys.captcha.enabled is false.
	captchaProvider, cleanup, err := newCaptchaProvider(sysConf.GetCaptcha(), redis)
	if err != nil {
		return nil, cleanup, err
	}

	d := coredata.NewData(adminDB)
	userAdminRepo := data.NewUserAdminRepo(d)
	loginLogger := data.NewLoginLogger(d)
	permCache := data.NewPermCache(redis)
	refRepo := data.NewRefRepo(d)
	brand := sysConf.GetBrand()

	// Cipher for OTP secrets: the key comes from sys.otp.aes_key (app.yaml commits a development placeholder; the
	// deployment layer injects the real one in production).
	cipher, err := provideOTPCipher(sysConf)
	if err != nil {
		return nil, cleanup, err
	}
	otpUseCase := biz.NewOTPUseCase(data.NewOTPRepo(d), cipher, brand.GetTitle(),
		data.NewOTPReplayGuard(redis, sysConf.GetOtp().GetMaxUses()))

	authUseCase, err := provideAuthUseCase(userAdminRepo, loginLogger, permCache, otpUseCase, sysConf, config,
		data.NewLockoutGuard(redis), captchaProvider)
	if err != nil {
		return nil, cleanup, err
	}
	httpAuth := config.GetServer().GetHttp()["default"].GetAuth()
	authService, err := service.NewAuthService(authUseCase, otpUseCase, userAdminRepo, refRepo,
		sysConf.GetSessionTtl().AsDuration(), httpAuth.GetCookieName(), sysConf.GetCookieSecure())
	if err != nil {
		return nil, cleanup, err
	}

	arbacRepo := data.NewARBACRepo(d)
	arbacUseCase := biz.NewARBACUseCase(arbacRepo, userAdminRepo)
	// otpUseCase satisfies biz.OTPResetter (Reset(ctx, userID) error): ResetOTP's checks and its effect both live in
	// UserAdminUseCase, with no detour through the service.
	userAdminUseCase := biz.NewUserAdminUseCase(userAdminRepo, arbacUseCase, otpUseCase)
	auditUseCase := biz.NewAuditUseCase(data.NewAuditRepo(d), loginLogger)

	return &Kernel{
		RefRepo:      refRepo,
		ScopeUseCase: biz.NewScopeUseCase(data.NewScopeRepo(d)),
		BusinessZone: businessZone,
		Brand:        brand,

		config: config,
		authUC: authUseCase,
		otpUC:  otpUseCase,
		audit:  auditUseCase,

		auth: authService,
		dept: service.NewDeptService(biz.NewDeptUseCase(data.NewDeptRepo(d), arbacUseCase)),
		role: service.NewRoleService(biz.NewRoleUseCase(data.NewRoleRepo(d), arbacRepo, permCache), refRepo),
		user: service.NewUserService(userAdminUseCase, userAdminRepo, refRepo),
		meta: service.NewMetaService(businessZone),
		logs: service.NewAuditService(auditUseCase),
	}, cleanup, nil
}

// RegisterKernelPermissions registers the kernel batch; the sys: business domain is reserved for it. di.go's
// registerPermissions calls it before the business batches.
func RegisterKernelPermissions() {
	perm.MustRegisterGroups(sys.Groups)
	perm.MustRegisterKernel(sys.Perms, service.Bindings, sys.Resources)
}

// Middleware puts the kernel's links into a chain built by mw.Default: principal (verified claims → principal),
// authz (permission-code bindings and step-up) and audit right after the validate slot, so after the authentication
// slot; locale right after metadata; sentinel (biz errors → typed API errors, kernel mappings plus the business's)
// last. Your own middleware goes in afterwards, before or after the named links "principal", "authz" and "audit".
func (k *Kernel) Middleware(chain *mw.Chain, sg *sentinel.Group) {
	insertKernelMiddleware(chain, k.authUC, k.otpUC, k.audit, sg)
}

// insertKernelMiddleware takes interfaces, so the chain order test runs the real insertion with test doubles.
func insertKernelMiddleware(chain *mw.Chain, resolver amw.PrincipalResolver, stepUp amw.StepUpVerifier, rec audit.Recorder, sg *sentinel.Group) {
	chain.InsertAfter(mw.SlotValidate, amw.Principal(resolver), "principal")
	chain.InsertAfter("principal", amw.Authz(stepUp), "authz")
	chain.InsertAfter("authz", amw.Audit(rec), "audit")
	mw.WithLocale(chain, i18n.NewBundle())
	mw.WithSentinel(chain, sg)
}

// NewHTTPServer seals the permission registry di.go filled and builds the default HTTP server: the kernel's services
// and then business's on the chain, the business raw routes (webhooks, downloads: http.Handlers that skip the chain),
// and the SPA last. Call it after every perm.MustRegister*.
//
// Seal checks that every operation of the mounted services is bound or anonymous: failing at startup beats an
// endpoint that answers 403 to everyone. The SPA goes last because it catches everything with HandlePrefix("/") and
// gorilla mux matches in registration order: before the raw routes it would shadow them, and a webhook would get the
// SPA's 404.
func (k *Kernel) NewHTTPServer(chain *mw.Chain, business []HTTPService, raw ...api.StreamRegister) (keel.Server, error) {
	served, err := ServedOperations(business)
	if err != nil {
		return nil, err
	}
	if err := perm.Seal(served); err != nil {
		return nil, err
	}
	services := append(kernelServices(k), business...)
	streams := append(append([]api.StreamRegister{}, raw...), RegisterSPA(k.Brand))
	return api.NewHTTPServer(k.config.GetServer().GetHttp()["default"], NewHTTPServerRegister(services...), chain, streams...)
}

// ServedOperations lists every operation of the kernel's and the business's HTTP services, the operations Seal
// checks. Only the services' names are read, so nil services are fine (cmd/<app>/registration_test.go).
func ServedOperations(business []HTTPService) ([]string, error) {
	services := append(kernelServices(nil), business...)
	names := make([]string, len(services))
	for i, svc := range services {
		names[i] = svc.Name
	}
	return perm.ServedOperations(names...)
}
