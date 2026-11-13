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

// ContractOperations lists every operation of this repository's proto services (packages starting with "api.", the
// layout of api/), sorted. di.go passes it to Seal so each one must be bound or anonymous.
func ContractOperations() []string {
	var ops []string
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if !strings.HasPrefix(string(fd.Package()), "api.") {
			return true
		}
		for i := 0; i < fd.Services().Len(); i++ {
			methods := fd.Services().Get(i).Methods()
			for j := 0; j < methods.Len(); j++ {
				ops = append(ops, parseFullMethodToOperation(methods.Get(j).FullName()))
			}
		}
		return true
	})
	slices.Sort(ops)
	return ops
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
