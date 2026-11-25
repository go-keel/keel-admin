package service

import (
	v1 "github.com/go-keel/keel-admin/api/admin_app/v1"
	"github.com/go-keel/keel-admin/internal/server"
	"github.com/go-keel/keel/server/api"
)

// HTTP services of the business area. Each business service gets one here, and `cmd/admin/di.go` lists them in
// `httpServices`, mounted after the kernel's own, so adding a business module does not touch `internal/server/http.go`.
//
// They live in the business area, not in internal/server: that is the kernel, and overwriting it as a whole would
// erase what the derived repo added.

// NoticeHTTP mounts the notice service on the HTTP server. Example: delete after forking (this comment and the
// function, together with the whole notice module)
func NoticeHTTP(svc *NoticeService) server.HTTPService {
	return server.HTTPService{
		Name:     v1.NoticeService_ServiceDesc.ServiceName,
		Register: func(s *api.HTTPServer) { v1.RegisterNoticeServiceHTTPServer(s, svc) },
	}
}
