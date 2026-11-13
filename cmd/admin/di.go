package main

import (
	"fmt"

	"github.com/go-keel/keel-admin/internal/core/errs"

	"github.com/go-keel/keel"
	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/captcha"
	base64captcha "github.com/go-keel/keel-admin/internal/core/captcha/base64"
	"github.com/go-keel/keel-admin/internal/core/captcha/turnstile"
	coredata "github.com/go-keel/keel-admin/internal/core/data"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/core/sys/data"
	"github.com/go-keel/keel-admin/internal/core/sys/service"

	// The business area's packages (internal/biz|data|service) share names with core/sys, so they are imported with
	// aliases.
	nbiz "github.com/go-keel/keel-admin/internal/biz"
	ndata "github.com/go-keel/keel-admin/internal/data"
	"github.com/go-keel/keel-admin/internal/server"
	nsvc "github.com/go-keel/keel-admin/internal/service"
	"github.com/go-keel/keel/core/kconfig"
	"github.com/go-keel/keel/core/kdb"
	"github.com/go-keel/keel/core/kotel"
	"github.com/go-keel/keel/core/kredis"
)

// wireApp assembles dependencies by hand: constructed in dependency order, cleaned up in reverse (LIFO).
func wireApp(bootstrap *kconfig.Dynamic[conf.Bootstrap]) (*keel.App, func(), error) {
	var cleanups []func()
	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}

	config, err := kconfig.NewConfig()
	if err != nil {
		return nil, cleanup, err
	}

	otelCleanup, err := kotel.Setup(config)
	if err != nil {
		return nil, cleanup, err
	}
	cleanups = append(cleanups, otelCleanup)

	keel.SetupLogger(config)
	appFactory := keel.NewAppFactory(keel.NewConfig(config))

	// Both databases are required, and a missing one fails right here: db = admin database (all sys_* tables),
	// bizDB = business database (the business area's own tables). default belongs to the derived repo's own code, and
	// the kernel, as a guest, takes a named instance; this and why there is no fallback: ADR admin-0002.
	// Data scope filtering (permdata.ApplyScope) builds WHERE only on the business tables' own owner columns, with no
	// cross-database JOIN, so two databases and one (both sources set to the same DSN) behave the same.
	db, dbCleanup, err := kdb.NewNamedDB(config, "admin")
	if err != nil {
		return nil, cleanup, err
	}
	cleanups = append(cleanups, dbCleanup)

	bizDB, bizCleanup, err := kdb.NewDB(config)
	if err != nil {
		return nil, cleanup, err
	}
	cleanups = append(cleanups, bizCleanup)

	redis, redisCleanup, err := kredis.NewRedis(config)
	if err != nil {
		return nil, cleanup, err
	}
	cleanups = append(cleanups, redisCleanup)

	// data
	dataData := coredata.NewData(db)
	userAdminRepo := data.NewUserAdminRepo(dataData)
	userRepo := data.NewUserRepo(userAdminRepo)
	loginLogger := data.NewLoginLogger(dataData)
	permCache := data.NewPermCache(redis)
	otpRepo := data.NewOTPRepo(dataData)

	// Framework config: the sys.* node of app.yaml, read once at startup (restart after a change).
	sysConf := server.ProvideSysConf(bootstrap)
	brand, err := server.ProvideBrand(sysConf)
	if err != nil {
		return nil, cleanup, err
	}
	businessZone, err := server.ProvideBusinessZone(sysConf)
	if err != nil {
		return nil, cleanup, err
	}
	cap := sysConf.GetCaptcha()
	lockoutGuard := data.NewLockoutGuard(redis)

	// captcha providers: both are registered unconditionally, and sys.captcha.provider decides which is used (the
	// selected one then checks its own settings with Ready, see biz.CaptchaGate). A new provider is registered here,
	// see internal/core/captcha.
	captcha.MustRegister("base64", base64captcha.New(redis, int(cap.GetBase64Length()), cap.GetBase64Ttl().AsDuration()))
	turnstileProvider, turnstileCleanup, err := turnstile.New(cap.GetTurnstile().GetSecretKey())
	if err != nil {
		return nil, cleanup, err
	}
	cleanups = append(cleanups, turnstileCleanup)
	captcha.MustRegister(turnstile.Name, turnstileProvider)

	// The selected provider must pass its self-check right here. Without it, turnstile missing a secret would show up
	// as "every login is rejected": fail-closed is right, but with no clue why, and that is exactly why
	// captcha.Provider.Ready exists. Only the selected one is checked: missing settings of an unselected provider
	// should not bring startup down.
	if cap.GetEnabled() {
		active, ok := captcha.Get(cap.GetProvider())
		if !ok {
			return nil, cleanup, errs.Rules.New("captcha.provider-unknown",
				fmt.Sprintf("sys.captcha.provider %q is not registered (registered: %v)", cap.GetProvider(), captcha.Names()),
				"set sys.captcha.provider to a registered provider, or register it with captcha.MustRegister in di.go")
		}
		if err := active.Ready(); err != nil {
			return nil, cleanup, errs.Rules.Wrap(err, "captcha.provider-not-ready",
				fmt.Sprintf("sys.captcha.provider %q is not ready: %v", cap.GetProvider(), err),
				"supply the provider's settings named above, or set sys.captcha.enabled to false")
		}
	}

	// Cipher for OTP secrets: the key comes from sys.otp.aes_key (app.yaml commits a development placeholder; the
	// deployment layer injects the real one in production).
	cipher, err := server.ProvideOTPCipher(sysConf)
	if err != nil {
		return nil, cleanup, err
	}
	otpReplayGuard := data.NewOTPReplayGuard(redis, sysConf.GetOtp().GetMaxUses())
	otpUseCase := server.ProvideOTPUseCase(otpRepo, cipher, brand, otpReplayGuard)
	otpGate := server.ProvideOTPGate(otpUseCase)

	// biz → service
	captchaGate := biz.NewCaptchaGate(cap.GetEnabled(), cap.GetProvider())
	authUseCase, err := server.ProvideAuthUseCase(userRepo, loginLogger, permCache, otpGate, sysConf, config, lockoutGuard, captchaGate)
	if err != nil {
		return nil, cleanup, err
	}
	httpAuth := config.GetServer().GetHttp()["default"].GetAuth()
	refRepo := data.NewRefRepo(dataData)
	authService, err := service.NewAuthService(authUseCase, otpUseCase, userAdminRepo, refRepo,
		sysConf.GetSessionTtl().AsDuration(), httpAuth.GetCookieName(), sysConf.GetCookieSecure())
	if err != nil {
		return nil, cleanup, err
	}

	roleRepo := data.NewRoleRepo(dataData)
	arbacRepo := data.NewARBACRepo(dataData)
	cacheInvalidator := data.NewCacheInvalidator(permCache)
	roleUseCase := biz.NewRoleUseCase(roleRepo, arbacRepo, cacheInvalidator)
	roleService := service.NewRoleService(roleUseCase, refRepo)

	arbacUseCase := biz.NewARBACUseCase(arbacRepo, userRepo)

	deptRepo := data.NewDeptRepo(dataData)
	deptUseCase := biz.NewDeptUseCase(deptRepo, arbacUseCase)
	deptService := service.NewDeptService(deptUseCase)

	// otpUseCase satisfies biz.OTPResetter (Reset(ctx, userID) error): ResetOTP's checks and its effect both live in
	// UserAdminUseCase, with no detour through the service.
	userAdminUseCase := biz.NewUserAdminUseCase(userAdminRepo, arbacUseCase, otpUseCase)
	userService := service.NewUserService(userAdminUseCase, userAdminRepo, refRepo)

	metaService := service.NewMetaService(businessZone)

	scopeRepo := data.NewScopeRepo(dataData)
	scopeUseCase := biz.NewScopeUseCase(scopeRepo)

	auditRepo := data.NewAuditRepo(dataData)
	auditUseCase := biz.NewAuditUseCase(auditRepo, loginLogger)
	auditService := service.NewAuditService(auditUseCase)

	// Example: delete after forking (up to noticeService)
	// notice: the first resident of the business area (internal/biz|data|service), proving that business stays out of
	// core/sys. Injecting scopeUseCase (core/sys/biz, implementing perm.ScopeResolver and perm.GrantRevoker) across
	// the business area / kernel boundary is legitimate: notice depends only on core/perm interfaces and does not
	// import core/sys. GrantRevoker cleans up explicit-assignment rows after a category is deleted (business code
	// never writes sys_* tables directly).
	noticeRepo := ndata.NewNoticeRepo(ndata.NewData(bizDB))
	noticeUseCase := nbiz.NewNoticeUseCase(noticeRepo, scopeUseCase, scopeUseCase)
	noticeService := nsvc.NewNoticeService(noticeUseCase, refRepo, businessZone)

	// Permission kernel assembly: after all services are constructed, register each domain's permission declarations,
	// then Seal freezes the registry; failing at startup beats finding configuration errors at runtime.
	registerPermissions(noticeRepo) // Example: delete after forking (the argument only)
	if err := perm.Seal(perm.ContractOperations()); err != nil {
		return nil, cleanup, err
	}
	// server: the kernel provides a list of registration functions and business modules append theirs; adding a
	// module adds one line here, without changing internal/server/http.go (kernel, overwritten as a whole).
	httpRegisters := server.KernelHTTPServers(
		authService, deptService, roleService, userService,
		metaService, auditService,
	)
	httpRegisters = append(httpRegisters, nsvc.NoticeHTTP(noticeService)) // Example: delete after forking
	httpServerRegister := server.NewHTTPServerRegister(httpRegisters...)
	auditRecorder := server.ProvideAuditRecorder(auditUseCase)
	sentinelGroup := nsvc.RegisterErrors(service.NewErrorSentinel())

	app, err := newApp(appFactory, httpServerRegister, authUseCase, otpUseCase, auditRecorder, sentinelGroup, brand)
	if err != nil {
		return nil, cleanup, err
	}
	return app, cleanup, nil
}

// registerPermissions registers every domain's permission declarations: the kernel batch, then one batch per business
// module. It is the only place a module registers its permissions; cmd/admin/registration_test.go seals the same
// registration, so adding a module changes nothing there.
//
// Groups (display names of business domains and feature modules) are registered separately from permissions: a group
// is only an organizational relation with no permission meaning, and several business modules may hang under the same
// business domain, each declaring its own part. Seal requires a name for the business domain and feature module of
// every code; a missing one fails startup.
func registerPermissions(
	noticeRepo nbiz.NoticeRepo, // Example: delete after forking
) {
	registerKernelPermissions()
	perm.MustRegisterGroups(nbiz.NoticeGroups) // Example: delete after forking
	// Example: delete after forking (up to perm.MustRegister)
	// notice is a business module: it registers the codes of the ops: business domain through perm.MustRegister (the
	// business entry, not MustRegisterKernel); Seal reserves the sys: business domain for the kernel batch.
	// NoticeResourceDefs(noticeRepo) fills the DeptOccupancy closure of the notice ResourceDef (querying
	// notice.owner_dept_id): the occupancy check when deleting a department (core/sys/biz.DeptUseCase.Delete) walks
	// the callbacks through perm.ResourcesWithDeptOccupancy(), so the kernel neither imports business packages nor
	// knows business table names.
	perm.MustRegister(nbiz.NoticePerms, nsvc.NoticeBindings, nbiz.NoticeResourceDefs(noticeRepo))
}

// registerKernelPermissions registers the kernel batch; the sys: business domain is reserved for it.
func registerKernelPermissions() {
	perm.MustRegisterGroups(sys.Groups)
	perm.MustRegisterKernel(sys.Perms, service.Bindings, sys.Resources)
}
