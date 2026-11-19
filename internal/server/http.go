package server

import (
	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
	"github.com/go-keel/keel-admin/internal/core/sys/service"
	"github.com/go-keel/keel/server/api"
)

// HTTPService is one proto service mounted on the HTTP server: its full name and its registration. Name comes from the
// generated <Service>_ServiceDesc.ServiceName, so a typo does not compile; newApp seals the permission registry against
// every operation of the named services, so a proto linked in only to call another service is never checked.
type HTTPService struct {
	Name     string
	Register api.HTTPServerRegister
}

// KernelHTTPServers returns the HTTP services of the kernel's six services. **Derived repos do not
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
) []HTTPService {
	return []HTTPService{
		{v1.AuthService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterAuthServiceHTTPServer(s, auth) }},
		{v1.DeptService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterDeptServiceHTTPServer(s, dept) }},
		{v1.RoleService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterRoleServiceHTTPServer(s, role) }},
		{v1.UserService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterUserServiceHTTPServer(s, user) }},
		{v1.MetaService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterMetaServiceHTTPServer(s, meta) }},
		{v1.AuditService_ServiceDesc.ServiceName, func(s *api.HTTPServer) { v1.RegisterAuditServiceHTTPServer(s, audit) }},
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

// ServiceNames lists the services' full names, the input of perm.ServedOperations.
func ServiceNames(services []HTTPService) []string {
	names := make([]string, len(services))
	for i, svc := range services {
		names[i] = svc.Name
	}
	return names
}
