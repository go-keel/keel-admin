package server

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/sys/service"
	"github.com/go-keel/keel/server/api"
)

// KernelHTTPServers returns the HTTP registration functions of the kernel's six services. **Derived repos do not
// edit this**: append your own after the returned slice (see the entry point's di.go); the registration functions
// of business modules live in the business area's own package.
// The entry directory name is not hard-coded here: this file is kernel and compared verbatim after normalization,
// while make rename CMD= replaces cmd/<entry name>, which normalization cannot restore; keeping it would make the
// derived repo's check-sync fail falsely.
//
// Business modules are not parameters of this function: otherwise every module a derived repo adds would change a
// line of the upstream parameter list and body, while an assembly seam only allows appending, never changing what
// is there.
func KernelHTTPServers(
	auth *service.AuthService,
	dept *service.DeptService,
	role *service.RoleService,
	user *service.UserService,
	meta *service.MetaService,
	audit *service.AuditService,
) []api.HTTPServerRegister {
	return []api.HTTPServerRegister{
		func(s *api.HTTPServer) { v1.RegisterAuthServiceHTTPServer(s, auth) },
		func(s *api.HTTPServer) { v1.RegisterDeptServiceHTTPServer(s, dept) },
		func(s *api.HTTPServer) { v1.RegisterRoleServiceHTTPServer(s, role) },
		func(s *api.HTTPServer) { v1.RegisterUserServiceHTTPServer(s, user) },
		func(s *api.HTTPServer) { v1.RegisterMetaServiceHTTPServer(s, meta) },
		func(s *api.HTTPServer) { v1.RegisterAuditServiceHTTPServer(s, audit) },
	}
}

// NewHTTPServerRegister combines several registration functions into one that runs them in the given order.
func NewHTTPServerRegister(regs ...api.HTTPServerRegister) api.HTTPServerRegister {
	return func(s *api.HTTPServer) {
		for _, reg := range regs {
			reg(s)
		}
	}
}
