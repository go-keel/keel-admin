package main

import (
	"time"

	_ "go.uber.org/automaxprocs"

	// Both drivers: database.<name>.driver picks one, per database. A repository on one database may delete the other line.
	_ "github.com/go-keel/keel/core/kdb/mysql"
	_ "github.com/go-keel/keel/core/kdb/postgres"

	"github.com/go-keel/keel-admin/internal/conf"
	"github.com/go-keel/keel/core/kconfig"
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
