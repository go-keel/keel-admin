package main

import (
	"time"

	_ "go.uber.org/automaxprocs"

	_ "github.com/go-keel/keel/core/kdb/mysql"

	"github.com/go-keel/keel"
	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel-admin/internal/core/audit"
	amw "github.com/go-keel/keel-admin/internal/core/middleware"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
	"github.com/go-keel/keel-admin/internal/i18n"
	"github.com/go-keel/keel-admin/internal/server"
	"github.com/go-keel/keel/core/kconfig"
	"github.com/go-keel/keel/middleware/sentinel"
	"github.com/go-keel/keel/mw"
	"github.com/go-keel/keel/server/api"
)

func main() {
	// Pin the process time zone to UTC: logs and time.Now readings do not change with the host's time zone setting
	// (ADR admin-0003).
	time.Local = time.UTC

	// If app.dev.yaml (gitignored) exists in the same directory it is overlaid on top: local development writes that
	// file, not the development placeholders committed here, which the deployment layer injects or replaces in
	// production.
	bootstrap, cleanup, err := kconfig.NewDynamic[conf.Bootstrap]("configs/app.yaml")
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// Register cleanup before checking the error: in the reverse order a failed start leaks resources.
	// Reason: docs/guides/assembly.md, section "The cmd's main.go skeleton".
	app, appCleanup, err := wireApp(bootstrap)
	if appCleanup != nil {
		defer appCleanup()
	}
	if err != nil {
		panic(err)
	}

	if err = app.Run(); err != nil {
		panic(err)
	}
}

func newApp(
	factory *keel.AppFactory,
	register api.HTTPServerRegister,
	authUC *biz.AuthUseCase,
	otpUC *biz.OTPUseCase,
	auditRec audit.Recorder,
	sg *sentinel.Group,
	brand *conf.Brand,
	// The business area's raw routes (http.Handlers that skip the middleware chain, such as webhooks and downloads)
	// are appended here.
	streamRegisters ...api.StreamRegister,
) (*keel.App, error) {
	c := factory.Config()
	httpCfg := c.Server.Http["default"]

	chain := mw.Default(c.Config)
	// mw.Default already reserves the authentication slot (between metadata and validate); api.NewHTTPServer fills it
	// with the authentication middleware built from httpCfg.GetAuth().
	// principal goes right after validate, so after the auth slot: claims can be resolved into a principal.
	chain.InsertAfter(mw.SlotValidate, amw.Principal(authUC), "principal") // verified claims -> permission principal
	chain.InsertAfter("principal", amw.Authz(otpUC), "authz")              // code-level permission bindings
	chain.InsertAfter("authz", amw.Audit(auditRec), "audit")
	mw.WithLocale(chain, i18n.NewBundle())
	mw.WithSentinel(chain, sg) // biz sentinel errors -> typed API errors

	// The SPA always goes last: it catches everything with HandlePrefix("/"), and gorilla mux matches in registration
	// order; placed before the business raw routes it would shadow them entirely, and a webhook would get the SPA's 404.
	streams := append(append([]api.StreamRegister{}, streamRegisters...), server.RegisterSPA(brand))
	httpSrv, err := api.NewHTTPServer(httpCfg, register, chain, streams...)
	if err != nil {
		return nil, err
	}
	return factory.New(httpSrv)
}
