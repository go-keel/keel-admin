package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v3/transport"
	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/go-keel/keel-admin/internal/core/perm"
	kauth "github.com/go-keel/keel/middleware/auth"
)

// fakeResolver is controllable: ResolveWithVersion always returns a copy of the principal, and RolePerms records
// its arguments and returns the preset set or error.
type fakeResolver struct {
	principal *perm.Principal
	perms     perm.Set
	permsErr  error
	gotRoles  []int64
}

var _ PrincipalResolver = (*fakeResolver)(nil)

func (f *fakeResolver) ResolveWithVersion(context.Context, int64, int) (*perm.Principal, error) {
	cp := *f.principal
	return &cp, nil
}

func (f *fakeResolver) RolePerms(_ context.Context, ids []int64) (perm.Set, error) {
	f.gotRoles = ids
	return f.perms, f.permsErr
}

// runPrincipal runs the Principal middleware once with verified sub=1 claims and returns the principal the handler
// sees. An empty previewHdr means no preview header.
func runPrincipal(t *testing.T, r *fakeResolver, previewHdr string) *perm.Principal {
	t.Helper()
	h := mapHeader{}
	if previewHdr != "" {
		h[PreviewHeader] = previewHdr
	}
	ctx := transport.NewServerContext(context.Background(), &fakeTransport{header: h})
	ctx = kauth.NewContext(ctx, jwtv5.MapClaims{"sub": "1", "ver": float64(0)})

	var got *perm.Principal
	next := func(ctx context.Context, req any) (any, error) {
		got, _ = perm.FromContext(ctx)
		return nil, nil
	}
	if _, err := Principal(r)(next)(ctx, nil); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	return got
}

// root with a preview header becomes a non-root principal with the chosen roles; UserID/DeptID stay its own.
func TestPreviewDemotesRoot(t *testing.T) {
	r := &fakeResolver{
		principal: &perm.Principal{UserID: 1, IsRoot: true, DeptID: 7, Perms: perm.Set{}},
		perms:     perm.Set{"sys:account:list": true},
	}
	p := runPrincipal(t, r, "2, 3")
	if p == nil || p.IsRoot {
		t.Fatalf("want demoted principal, got %+v", p)
	}
	if p.UserID != 1 || p.DeptID != 7 {
		t.Errorf("identity changed: %+v", p)
	}
	if len(r.gotRoles) != 2 || r.gotRoles[0] != 2 || r.gotRoles[1] != 3 {
		t.Errorf("roles = %v, want [2 3]", r.gotRoles)
	}
	if !p.Perms.Has("sys:account:list") {
		t.Errorf("perms not applied: %+v", p.Perms)
	}
}

// A non-root user with a preview header is ignored: the header only reduces permissions and never changes an
// ordinary user's.
func TestPreviewIgnoredForNonRoot(t *testing.T) {
	r := &fakeResolver{
		principal: &perm.Principal{UserID: 1, RoleIDs: []int64{9}, Perms: perm.Set{"a": true}},
	}
	p := runPrincipal(t, r, "2,3")
	if r.gotRoles != nil {
		t.Errorf("RolePerms called for non-root: %v", r.gotRoles)
	}
	if len(p.RoleIDs) != 1 || p.RoleIDs[0] != 9 || !p.Perms.Has("a") {
		t.Errorf("principal changed: %+v", p)
	}
}

// root without a preview header, or with no valid ID in it, stays root.
func TestPreviewAbsentOrGarbage(t *testing.T) {
	for _, hdr := range []string{"", "abc", "0,-1", ",,"} {
		r := &fakeResolver{principal: &perm.Principal{UserID: 1, IsRoot: true}}
		if p := runPrincipal(t, r, hdr); p == nil || !p.IsRoot {
			t.Errorf("hdr %q: root lost: %+v", hdr, p)
		}
	}
}

// A RolePerms error keeps root as is (role codes are cached, failures are rare, and the subject is root anyway).
func TestPreviewResolveErrorKeepsRoot(t *testing.T) {
	r := &fakeResolver{
		principal: &perm.Principal{UserID: 1, IsRoot: true},
		permsErr:  errors.New("db down"),
	}
	if p := runPrincipal(t, r, "2"); p == nil || !p.IsRoot {
		t.Errorf("root lost on resolve error: %+v", p)
	}
}
