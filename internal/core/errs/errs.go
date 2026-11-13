// Package errs holds the admin kernel's rule namespace: developer-facing errors from the kernel (permission registry,
// kernel config, captcha wiring) are built with Rules.New / Rules.Wrap, each ID documented in
// docs/admin/startup-errors.md.
package errs

import "github.com/go-keel/keel/pkg/rule"

// Rules is the kernel's rule namespace.
var Rules = rule.Namespace{Name: "keel-admin", Doc: "docs/admin/startup-errors.md"}
