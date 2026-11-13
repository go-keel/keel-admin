package service

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel/server/api"
)

// HTTP registration functions of the business area. Each business service gets one here, and `cmd/admin/di.go`
// appends them after the kernel's list (`server.KernelHTTPServers`), so adding a business module does not touch
// `internal/server/http.go`.
//
// They live in the business area, not in internal/server: that is the kernel, and overwriting it as a whole would
// erase what the derived repo added.

// NoticeHTTP mounts the notice service on the HTTP server.
// Example: delete after forking (together with the whole notice module)
func NoticeHTTP(svc *NoticeService) api.HTTPServerRegister {
	return func(s *api.HTTPServer) { v1.RegisterNoticeServiceHTTPServer(s, svc) }
}
