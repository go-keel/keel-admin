package perm

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	keelpb "github.com/go-keel/keel/proto/keel/v1"

	"github.com/go-keel/keel-admin/internal/core/errs"
)

// AnonymousOperations scans protoregistry.GlobalFiles for the keel.v1.auth
// extension and returns the set of operations (e.g.
// "/api.keel_admin.v1.AuthService/Login") annotated with allow_anonymous. Proto
// annotations are the single source of truth for the anonymous whitelist —
// there is no code-level binding for it. Lazily built and cached on first use.
var AnonymousOperations = sync.OnceValue(func() map[string]struct{} {
	ops := make(map[string]struct{})

	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			service := fd.Services().Get(i)
			for j := 0; j < service.Methods().Len(); j++ {
				method := service.Methods().Get(j)
				opts := method.Options().(*descriptorpb.MethodOptions)
				authRule, _ := proto.GetExtension(opts, keelpb.E_Auth).(*keelpb.AuthRule)
				if authRule != nil && authRule.AllowAnonymous {
					ops[parseFullMethodToOperation(method.FullName())] = struct{}{}
				}
			}
		}
		return true
	})

	return ops
})

// ServedOperations lists every operation (e.g. "/api.keel_admin.v1.UserService/ListUser") of the named proto services,
// sorted. The names come from the HTTP registration list (server.HTTPService), the same list that mounts the routes,
// so only services this process serves are checked: a proto linked in only to call another service is not named.
// Every method counts, whatever its HTTP binding or streaming shape. server.Kernel.NewHTTPServer passes the result to Seal so each one must
// be bound or anonymous.
func ServedOperations(services ...string) ([]string, error) {
	var ops []string
	for _, name := range services {
		d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(name))
		sd, ok := d.(protoreflect.ServiceDescriptor)
		if err != nil || !ok {
			return nil, errs.Rules.New("perm.service-unknown", fmt.Sprintf("HTTP service %q is not a proto service linked into this binary", name), "set HTTPService.Name from the generated <Service>_ServiceDesc.ServiceName")
		}
		for j := 0; j < sd.Methods().Len(); j++ {
			ops = append(ops, parseFullMethodToOperation(sd.Methods().Get(j).FullName()))
		}
	}
	slices.Sort(ops)
	return slices.Compact(ops), nil
}

// IsAnonymous reports whether op (e.g. "/api.keel_admin.v1.AuthService/Login") is
// annotated allow_anonymous in its proto definition.
func IsAnonymous(op string) bool {
	_, ok := AnonymousOperations()[op]
	return ok
}

func parseFullMethodToOperation(fullName protoreflect.FullName) string {
	// "api.keel_admin.v1.AuthService.Login"
	name := string(fullName)
	index := strings.LastIndex(name, ".")
	if index < 0 {
		return ""
	}
	service := name[:index]  // api.keel_admin.v1.AuthService
	method := name[index+1:] // Login
	return fmt.Sprintf("/%s/%s", service, method)
}
