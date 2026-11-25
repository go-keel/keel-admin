package server

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel/server/api"
)

// HTTPService is one proto service mounted on the HTTP server: its full name and its registration. Name comes from the
// generated <Service>_ServiceDesc.ServiceName, so a typo does not compile; Kernel.NewHTTPServer seals the permission
// registry against every operation of the named services, so a proto linked in only to call another service is never
// checked. A business module's entry lives in the business area's internal/service/http.go.
type HTTPService struct {
	Name     string
	Register api.HTTPServerRegister
}

// kernelServices returns the kernel's six HTTP services. The registrations read k only when they run, so
// kernelServices(nil) is enough to list the names.
func kernelServices(k *Kernel) []HTTPService {
	return []HTTPService{
		{v1.AuthService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterAuthServiceHTTPServer(s, k.auth) }},
		{v1.DeptService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterDeptServiceHTTPServer(s, k.dept) }},
		{v1.RoleService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterRoleServiceHTTPServer(s, k.role) }},
		{v1.UserService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterUserServiceHTTPServer(s, k.user) }},
		{v1.MetaService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterMetaServiceHTTPServer(s, k.meta) }},
		{v1.AuditService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterAuditServiceHTTPServer(s, k.logs) }},
	}
}

// NewHTTPServerRegister combines the services' registrations into one that runs them in the given order.
func NewHTTPServerRegister(services ...HTTPService) api.HTTPServerRegister {
	return func(s *api.HTTPServer) {
		for _, svc := range services {
			svc.Register(s)
		}
	}
}
