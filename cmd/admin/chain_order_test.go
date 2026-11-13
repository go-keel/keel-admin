package main

// Authentication must run before principal/authz/audit: all three middlewares depend on the claims authentication
// writes into ctx. The wrong order is no compile error; it only shows at runtime as "logged in, yet treated as
// logged out".
//
// How it asserts: build the chain in the same order as newApp, start a server on a real port with the real
// api.NewHTTPServer, and send real HTTP requests. principal calls the PrincipalResolver only after reading claims from
// ctx (see internal/core/middleware/principal.go), so a call to the resolver proves authentication ran first;
// conversely, a request without credentials should be rejected at the authentication slot, and the resolver is never
// called.
//
// It replicates the few chain-building lines of newApp instead of calling it: newApp's authUC/otpUC parameters are
// concrete types that need a real database, while principal/authz/audit take interfaces that accept test doubles.
import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	khttp "github.com/go-kratos/kratos/v3/transport/http"

	"github.com/go-keel/keel-admin/internal/core/audit"
	amw "github.com/go-keel/keel-admin/internal/core/middleware"
	"github.com/go-keel/keel-admin/internal/core/perm"
	"github.com/go-keel/keel-admin/internal/i18n"
	kconf "github.com/go-keel/keel/conf"
	"github.com/go-keel/keel/middleware/sentinel"
	"github.com/go-keel/keel/mw"
	"github.com/go-keel/keel/pkg/jwt"
	"github.com/go-keel/keel/server/api"
)

const chainOrderTestSecret = "chain-order-test-secret"
const chainOrderTestOp = "/test/Probe"

// fakePrincipalResolver records whether ResolveWithVersion was called and the userID it received: the principal
// middleware calls it only after reading the claims authentication wrote into ctx and extracting sub. It returns an
// IsRoot principal so that everything after authz lets the request through to the handler.
type fakePrincipalResolver struct {
	mu     sync.Mutex
	called bool
	gotID  int64
}

func (f *fakePrincipalResolver) ResolveWithVersion(_ context.Context, userID int64, _ int) (*perm.Principal, error) {
	f.mu.Lock()
	f.called = true
	f.gotID = userID
	f.mu.Unlock()
	return &perm.Principal{UserID: userID, IsRoot: true}, nil
}

func (f *fakePrincipalResolver) RolePerms(context.Context, []int64) (perm.Set, error) {
	return nil, nil
}

func (f *fakePrincipalResolver) wasCalled() (bool, int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.called, f.gotID
}

// fakeStepUp: this test does not trigger the RequireOTP branch, so neither method is used.
type fakeStepUp struct{}

func (fakeStepUp) Verify(context.Context, int64, string) (bool, error) { return false, nil }
func (fakeStepUp) Bound(context.Context, int64) (bool, error)          { return false, nil }

// fakeAuditRecorder: the Requirement in this test has no Audit, so Record is never called.
type fakeAuditRecorder struct{}

func (fakeAuditRecorder) Record(context.Context, audit.Entry) {}

// buildTestChain matches the chain-building code in newApp line by line (see main.go), with authUC/otpUC/auditRec
// replaced by test doubles.
func buildTestChain(resolver amw.PrincipalResolver) *mw.Chain {
	chain := mw.Default(nil)
	chain.InsertAfter(mw.SlotValidate, amw.Principal(resolver), "principal")
	chain.InsertAfter("principal", amw.Authz(fakeStepUp{}), "authz")
	chain.InsertAfter("authz", amw.Audit(fakeAuditRecorder{}), "audit")
	mw.WithLocale(chain, i18n.NewBundle())
	mw.WithSentinel(chain, sentinel.NewGroup())
	return chain
}

// registerProbeRoute hand-writes a route shaped like a contract endpoint, without depending on protoc-generated code.
func registerProbeRoute(srv *api.HTTPServer) {
	srv.Route("/").GET("/probe", func(ctx khttp.Context) error {
		khttp.SetOperation(ctx, chainOrderTestOp)
		h := ctx.Middleware(func(c context.Context, req any) (any, error) {
			return "ok", nil
		})
		out, err := h(ctx, &struct{}{})
		if err != nil {
			return err
		}
		return ctx.Result(200, out)
	})
}

// startChainOrderServer starts an HTTP server on a real port with the real api.NewHTTPServer; the chain is built the
// same way as in production newApp, and auth is built from cfg.Auth (HS256).
func startChainOrderServer(t *testing.T, chain *mw.Chain) (base string) {
	t.Helper()
	cfg := &kconf.Server_HTTP{
		Addr: "127.0.0.1:0",
		Auth: &kconf.Server_Auth{
			Enabled:   true,
			Algorithm: "HS256",
			JwtSecret: chainOrderTestSecret,
		},
	}
	srvT, err := api.NewHTTPServer(cfg, registerProbeRoute, chain)
	if err != nil {
		t.Fatalf("api.NewHTTPServer: %v", err)
	}
	srv, ok := srvT.(*api.HTTPServer)
	if !ok {
		t.Fatalf("unexpected server type %T", srvT)
	}
	endpoint, err := srv.Endpoint()
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	go func() { _ = srv.Start(context.Background()) }()
	t.Cleanup(func() { _ = srv.Stop(context.Background()) })
	// Wait until it really listens: Endpoint() returns a valid address only after Start triggers listen, but the client
	// may still send the first request before the accept loop is ready; retry a few times so the test is not flaky.
	for i := 0; i < 50; i++ {
		if resp, err := http.Get("http://" + endpoint.Host + "/probe"); err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "http://" + endpoint.Host
}

func signChainOrderToken(t *testing.T, userID int64) string {
	t.Helper()
	claims := jwt.NewClaims(strconv.FormatInt(userID, 10), time.Hour)
	claims["ver"] = float64(0)
	signer, err := jwt.NewHS256Signer([]byte(chainOrderTestSecret))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

// A call to the resolver proves authentication runs before principal; authz/audit are placed after principal with
// chain.InsertAfter, so the same conclusion covers them.
func TestChainOrder_AuthRunsBeforePrincipalAuthzAudit(t *testing.T) {
	perm.ResetForTest()
	t.Cleanup(perm.ResetForTest)
	perm.MustRegisterKernel(nil, perm.Bindings{chainOrderTestOp: {}}, nil)
	if err := perm.Seal(nil); err != nil {
		t.Fatalf("perm.Seal: %v", err)
	}

	resolver := &fakePrincipalResolver{}
	base := startChainOrderServer(t, buildTestChain(resolver))

	t.Run("valid token: principal resolves claims written by auth", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, base+"/probe", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+signChainOrderToken(t, 42))

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 (chain: auth -> principal(root bypass) -> authz -> audit -> handler)", resp.StatusCode)
		}
		called, gotID := resolver.wasCalled()
		if !called {
			t.Fatal("PrincipalResolver.ResolveWithVersion was never called — principal did not see auth's claims, order is broken")
		}
		if gotID != 42 {
			t.Fatalf("resolver got userID = %d, want 42 (claims sub not decoded correctly)", gotID)
		}
	})

	t.Run("no credentials: auth rejects before principal ever runs", func(t *testing.T) {
		resolver2 := &fakePrincipalResolver{}
		base2 := startChainOrderServer(t, buildTestChain(resolver2))

		resp, err := http.Get(base2 + "/probe")
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if called, gotID := resolver2.wasCalled(); called {
			t.Fatalf("PrincipalResolver.ResolveWithVersion was called (userID=%d) despite missing credentials — auth did not run first, or did not actually block the request", gotID)
		}
	})
}
