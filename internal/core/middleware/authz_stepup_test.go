package middleware

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/go-kratos/kratos/v3/errors"
	"github.com/go-kratos/kratos/v3/transport"

	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/core/sys/biz"
)

// fakeStepUp is controllable: whether an authenticator is bound, whether the code is valid, whether a lookup fails,
// whether the code is replayed.
type fakeStepUp struct {
	bound     bool
	valid     bool
	boundErr  error
	verifyErr error
}

var _ StepUpVerifier = (*fakeStepUp)(nil)

func (f *fakeStepUp) Verify(context.Context, int64, string) (bool, error) {
	return f.valid, f.verifyErr
}
func (f *fakeStepUp) Bound(context.Context, int64) (bool, error) {
	return f.bound, f.boundErr
}

// fakeTransport provides the Operation and request headers, satisfying transport.Transporter.
type fakeTransport struct {
	transport.Transporter
	op     string
	header transport.Header
}

func (t *fakeTransport) Operation() string               { return t.op }
func (t *fakeTransport) RequestHeader() transport.Header { return t.header }

type mapHeader map[string]string

func (h mapHeader) Get(k string) string    { return h[k] }
func (h mapHeader) Set(k, v string)        { h[k] = v }
func (h mapHeader) Add(k, v string)        { h[k] = v }
func (h mapHeader) Keys() []string         { return nil }
func (h mapHeader) Values(string) []string { return nil }

const stepUpOp = "/api.keel_admin.v1.RoleService/SetRolePermissions"

// runAuthz runs authorization for one step-up operation and returns whether the handler was reached and the error.
func runAuthz(t *testing.T, gate *fakeStepUp, otpHeader string) (called bool, err error) {
	t.Helper()
	perm.ResetForTest()
	t.Cleanup(perm.ResetForTest)
	perm.MustRegisterKernel(nil, perm.Bindings{
		stepUpOp: {RootOnly: true, RequireOTP: true},
	}, nil)
	if serr := perm.Seal(nil); serr != nil {
		t.Fatalf("seal: %v", serr)
	}

	h := mapHeader{}
	if otpHeader != "" {
		h[stepUpHeader] = otpHeader
	}
	ctx := transport.NewServerContext(context.Background(),
		&fakeTransport{op: stepUpOp, header: h})
	// root, so any rejection comes from step-up rather than a permission-code mismatch.
	ctx = perm.NewContext(ctx, &perm.Principal{UserID: 1, IsRoot: true})

	next := func(ctx context.Context, req any) (any, error) {
		called = true
		return "ok", nil
	}
	_, err = Authz(gate)(next)(ctx, nil)
	return called, err
}

// Someone without a bound authenticator cannot supply any code: answering OTP_STEPUP would make the operation unusable
// for them forever, so the request must be routed to the binding guide.
func TestStepUpUnboundGetsBindRequired(t *testing.T) {
	called, err := runAuthz(t, &fakeStepUp{bound: false}, "")

	if called {
		t.Fatal("unbound should not pass")
	}
	if errors.Reason(err) != "OTP_BIND_REQUIRED" {
		t.Fatalf("unbound should return OTP_BIND_REQUIRED so the frontend opens the binding guide, got %v", err)
	}
}

// Bound but no code sent: this is the real OTP_STEPUP, and the frontend just asks for the code.
func TestStepUpBoundWithoutCodeGetsStepUp(t *testing.T) {
	called, err := runAuthz(t, &fakeStepUp{bound: true}, "")

	if called {
		t.Fatal("a missing step-up code should not pass")
	}
	if errors.Reason(err) != "OTP_STEPUP" {
		t.Fatalf("bound without a code should return OTP_STEPUP, got %v", err)
	}
}

// A code was sent but is wrong: this must differ from "no code sent". The OTP_STEPUP text is "enter the code", and a
// user who mistyped one digit and is told that only keeps checking that they did fill it in (this really happened).
func TestStepUpBoundWithBadCodeRejected(t *testing.T) {
	called, err := runAuthz(t, &fakeStepUp{bound: true, valid: false}, "000000")

	if called {
		t.Fatal("a wrong step-up code should not pass")
	}
	if errors.Reason(err) != "OTP_INVALID" {
		t.Fatalf("a wrong code should return OTP_INVALID (not OTP_STEPUP), got %v", err)
	}
}

// The verification itself failed (guard unavailable, key cannot be decrypted): neither a wrong code nor a missing one.
// Reporting either of those tells the user they made a mistake, while the fault is on the server.
func TestStepUpVerifyFailureReportedAsServerFault(t *testing.T) {
	called, err := runAuthz(t,
		&fakeStepUp{bound: true, verifyErr: stderrors.New("guard down")}, "123456")

	if called {
		t.Fatal("a verification fault should not pass")
	}
	if errors.Reason(err) != "UNEXPECTED_ERROR" {
		t.Fatalf("a verification fault should return UNEXPECTED_ERROR (not OTP_STEPUP / OTP_INVALID), got %v", err)
	}
	if errors.Code(err) != 500 {
		t.Fatalf("a server fault should return 500, not 401, got %d", errors.Code(err))
	}
}

func TestStepUpBoundWithValidCodePasses(t *testing.T) {
	called, err := runAuthz(t, &fakeStepUp{bound: true, valid: true}, "123456")

	if err != nil {
		t.Fatalf("a correct step-up code should pass: %v", err)
	}
	if !called {
		t.Fatal("a correct step-up code should reach the handler")
	}
}

// The code is right but already used (two step-ups in a row within 90s show the same code): this must differ from a
// wrong code, otherwise a user staring at the correct code in the app is told it is wrong and assumes the system is
// broken.
func TestStepUpReplayedCodeReportedAsAlreadyUsed(t *testing.T) {
	called, err := runAuthz(t,
		&fakeStepUp{bound: true, verifyErr: biz.ErrOTPUsed}, "123456")

	if called {
		t.Fatal("a replayed code should not pass")
	}
	if errors.Reason(err) != "OTP_ALREADY_USED" {
		t.Fatalf("a replay should return OTP_ALREADY_USED (not OTP_STEPUP), got %v", err)
	}
}

// A Bound lookup error must fail closed: better to block the sensitive operation than to degrade to "no step-up
// needed". Someone who sent a code especially must not be told "enter the code": they did, and not knowing whether
// they are bound is the server's problem.
func TestStepUpBoundQueryErrorFailsClosed(t *testing.T) {
	called, err := runAuthz(t,
		&fakeStepUp{boundErr: errors.InternalServer("DB", "down"), valid: true}, "123456")

	if called {
		t.Fatal("a Bound lookup error must never let a sensitive operation through")
	}
	if errors.Reason(err) != "UNEXPECTED_ERROR" {
		t.Fatalf("a Bound lookup error should return UNEXPECTED_ERROR, got %v", err)
	}
}
