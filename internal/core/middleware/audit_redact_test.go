package middleware

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-keel/keel/pkg/redact"

	v1 "github.com/go-keel/keel-admin/api/keel_admin/v1"
)

// TestRequestJSONRedactsSecrets: request_payload comes from the request body, and the bodies of CreateUser /
// ResetPassword hold plaintext passwords. The audit table is kept long-term and root can read all of it, so copying a
// password into it is far worse than an audit that is not detailed enough; this test keeps redaction from being
// bypassed.
//
// It is also the end-to-end acceptance of the proto annotation path: the annotation is written in the .proto and
// resolved at runtime through keelpb's extension type, so this test fails if any link in between is missing.
func TestRequestJSONRedactsSecrets(t *testing.T) {
	got := requestJSON(&v1.CreateUserRequest{
		Username: "alice",
		Password: "sup3r-s3cret",
		Nickname: "Alice",
	})
	if got == nil {
		t.Fatal("requestJSON = nil, want JSON")
	}
	if strings.Contains(*got, "sup3r-s3cret") {
		t.Fatalf("plaintext password reached request_payload: %s", *got)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(*got), &obj); err != nil {
		t.Fatalf("request_payload is not valid JSON (the json column would reject the insert with 3140): %v", err)
	}
	if obj["password"] != redact.Mask {
		t.Errorf("password = %v, want %q", obj["password"], redact.Mask)
	}
	// Non-sensitive fields must stay, otherwise the audit records nothing.
	if obj["username"] != "alice" {
		t.Errorf("username = %v, want alice (redaction must not hit ordinary fields)", obj["username"])
	}
}

func TestRequestJSONResetPassword(t *testing.T) {
	got := requestJSON(&v1.ResetPasswordRequest{Id: 7, Password: "new-one"})
	if got == nil || strings.Contains(*got, "new-one") {
		t.Fatalf("the new password of a password reset reached request_payload: %v", got)
	}
}

// TestRequestJSONFieldNamedCode: the field holding a one-time OTP code is just called code. Guessing sensitivity from
// field names would never catch it, while the annotation does. This test guards the trade-off "the basis is in the
// contract, not in the name".
func TestRequestJSONFieldNamedCode(t *testing.T) {
	got := requestJSON(&v1.ConfirmOTPRequest{Code: "123456"})
	if got == nil || strings.Contains(*got, "123456") {
		t.Fatalf("OTP code reached request_payload: %v", got)
	}
}

// TestRequestJSONNonProto: a non-proto request returns nil, stored as SQL NULL. It must not return an empty string:
// "" is not a valid JSON document and MySQL's json column rejects it, and since Record is best-effort (failures are
// only logged), the audit would silently disappear.
func TestRequestJSONNonProto(t *testing.T) {
	if got := requestJSON(struct{ A int }{1}); got != nil {
		t.Fatalf("requestJSON(non-proto) = %v, want nil", *got)
	}
}
