package main

import (
	"github.com/go-keel/keel"
	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/perm"
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
	"github.com/go-keel/keel/mw"
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

	// The kernel: the sys_* domain on the admin database, its services, captcha and the permission middleware's
	// dependencies. sys.* of app.yaml is read once here (restart after a change).
	kernel, kernelCleanup, err := server.NewKernel(config, bootstrap.Get().Sys, db, redis)
	cleanups = append(cleanups, kernelCleanup)
	if err != nil {
		return nil, cleanup, err
	}

	// Example: delete after forking (up to noticeService)
	// notice: the first resident of the business area (internal/biz|data|service), proving that business stays out of
	// core/sys. Injecting kernel.ScopeUseCase (implementing perm.ScopeResolver and perm.GrantRevoker) across the
	// business area / kernel boundary is legitimate: notice depends only on core/perm interfaces and does not import
	// core/sys. GrantRevoker cleans up explicit-assignment rows after a category is deleted (business code never
	// writes sys_* tables directly).
	noticeRepo := ndata.NewNoticeRepo(ndata.NewData(bizDB))
	noticeUseCase := nbiz.NewNoticeUseCase(noticeRepo, kernel.ScopeUseCase, kernel.ScopeUseCase)
	noticeService := nsvc.NewNoticeService(noticeUseCase, kernel.RefRepo, kernel.BusinessZone)

	// After all services are constructed, register each domain's permission declarations; kernel.NewHTTPServer seals
	// the registry, checking every operation of the HTTP services it mounts.
	registerPermissions(noticeRepo) // Example: delete after forking (the argument only)

	// The chain: Keel's defaults, then the kernel's links (principal, authz, audit, locale, sentinel); your own
	// middleware goes in after this call, before or after a named link (docs/admin/assembly.md).
	chain := mw.Default(config)
	kernel.Middleware(chain, nsvc.RegisterErrors(service.NewErrorSentinel()))

	// The business area's raw routes (http.Handlers that skip the chain, such as webhooks and downloads) are passed
	// after httpServices(...) as further arguments; the kernel mounts the SPA after them.
	httpSrv, err := kernel.NewHTTPServer(chain, httpServices(
		noticeService, // Example: delete after forking
	))
	if err != nil {
		return nil, cleanup, err
	}
	app, err := keel.NewApp(config, httpSrv)
	if err != nil {
		return nil, cleanup, err
	}
	return app, cleanup, nil
}

// httpServices lists the business area's HTTP services, mounted after the kernel's: adding a module adds one parameter
// and one line here. The same list names what the permission check covers; cmd/admin/registration_test.go seals it
// too.
func httpServices(
	notice *nsvc.NoticeService, // Example: delete after forking
) []server.HTTPService {
	var services []server.HTTPService
	services = append(services, nsvc.NoticeHTTP(notice)) // Example: delete after forking
	return services
}

// registerPermissions registers every domain's permission declarations: the kernel batch, then one batch per business
// module. It is the only place a module registers its permissions; cmd/admin/registration_test.go seals the same
// registration, and a module passes nil there for each parameter it adds.
//
// Groups (display names of business domains and feature modules) are registered separately from permissions: a group
// is only an organizational relation with no permission meaning, and several business modules may hang under the same
// business domain, each declaring its own part. Seal requires a name for the business domain and feature module of
// every code; a missing one fails startup.
func registerPermissions(
	noticeRepo nbiz.NoticeRepo, // Example: delete after forking
) {
	server.RegisterKernelPermissions()
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
