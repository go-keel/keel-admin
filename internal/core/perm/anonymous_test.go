package perm

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	_ "google.golang.org/protobuf/types/known/emptypb"

	"github.com/go-keel/keel/pkg/rule"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
)

func TestAnonymousFromProto(t *testing.T) {
	// The admin v1 proto is imported (the test file blank-imports the api package); Login carries allow_anonymous.
	if !IsAnonymous("/api.keel_admin.v1.AuthService/Login") {
		t.Fatal("Login should be in the anonymous set (proto annotation)")
	}
	if IsAnonymous("/api.keel_admin.v1.AuthService/Profile") {
		t.Fatal("Profile should not be anonymous")
	}
}

// registerReviewProto links a proto the way a called-only contract is linked: its descriptors are in the process, its
// routes are not mounted. Stream.Upload is a client-streaming method.
var registerReviewProto = sync.OnceFunc(func() {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("review/remote.proto"),
		Package:    proto.String("review"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/protobuf/empty.proto"},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{Name: proto.String("Remote"), Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("List"), InputType: proto.String(".google.protobuf.Empty"),
				OutputType: proto.String(".google.protobuf.Empty"),
			}}},
			{Name: proto.String("Stream"), Method: []*descriptorpb.MethodDescriptorProto{{
				Name: proto.String("Upload"), InputType: proto.String(".google.protobuf.Empty"),
				OutputType: proto.String(".google.protobuf.Empty"), ClientStreaming: proto.Bool(true),
			}}},
		},
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		panic(err)
	}
	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		panic(err)
	}
})

func TestServedOperations(t *testing.T) {
	registerReviewProto()

	t.Run("only named services count", func(t *testing.T) {
		desc := v1.DeptService_ServiceDesc
		ops, err := ServedOperations(desc.ServiceName)
		if err != nil {
			t.Fatal(err)
		}
		want := len(desc.Methods) + len(desc.Streams)
		if len(ops) != want {
			t.Errorf("ServedOperations = %d operations, want all %d of DeptService: %v", len(ops), want, ops)
		}
		for _, op := range ops {
			if !strings.HasPrefix(op, "/"+desc.ServiceName+"/") {
				t.Errorf("ServedOperations lists %s, which is not mounted", op)
			}
		}
		if slices.Contains(ops, "/review.Remote/List") {
			t.Error("a called-only service was listed")
		}
	})

	t.Run("streaming methods count like any other", func(t *testing.T) {
		ops, err := ServedOperations("review.Stream")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ops, []string{"/review.Stream/Upload"}) {
			t.Errorf("ServedOperations(review.Stream) = %v, want the client-streaming Upload", ops)
		}
	})

	t.Run("an unknown name is a rule error", func(t *testing.T) {
		_, err := ServedOperations("review.NoSuchService")
		var re *rule.Error
		if !errors.As(err, &re) || re.ID != "perm.service-unknown" {
			t.Fatalf("ServedOperations(unknown) err = %v, want perm.service-unknown", err)
		}
	})
}
