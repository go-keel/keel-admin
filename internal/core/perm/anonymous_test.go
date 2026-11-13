package perm

import (
	"testing"

	_ "github.com/go-keel/keel-admin/api/keel_admin/v1"
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
