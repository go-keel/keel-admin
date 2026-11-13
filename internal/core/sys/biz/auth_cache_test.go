package biz

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeUserRepo is a minimal UserRepo fake for Resolve/cache aggregation tests.
type fakeUserRepo struct {
	user      *User
	roles     []int64
	roleCodes map[int64][]string
	bumpCalls []int64
	// accountChanges records every UpdateAccount: revoking sessions must be sent together with the same write.
	accountChanges []AccountChange

	// settings + mu replicate the repo's row-lock semantics: the whole read-modify-write is mutually exclusive, so
	// two concurrent favorite adds each read the other's finished result. Without this lock the UpdateSettings
	// concurrency test would test nothing.
	mu       sync.Mutex
	settings UserSettings

	findByUsernameErr error // when non-nil, FindByUsername returns it directly
}

var _ UserRepo = (*fakeUserRepo)(nil)

func (f *fakeUserRepo) FindByUsername(context.Context, string) (*User, error) {
	if f.findByUsernameErr != nil {
		return nil, f.findByUsernameErr
	}
	return f.user, nil
}
func (f *fakeUserRepo) FindByID(_ context.Context, id int64) (*User, error) {
	if f.user != nil {
		return f.user, nil
	}
	return &User{ID: id}, nil
}
func (f *fakeUserRepo) RoleIDs(context.Context, int64) ([]int64, error) { return f.roles, nil }
func (f *fakeUserRepo) RolePermCodesByRole(_ context.Context, roleID int64) ([]string, error) {
	return f.roleCodes[roleID], nil
}
func (f *fakeUserRepo) SetProfile(context.Context, int64, ProfileEdit) error { return nil }
func (f *fakeUserRepo) UpdateAccount(_ context.Context, id int64, c AccountChange) error {
	f.accountChanges = append(f.accountChanges, c)
	if c.RevokeSessions {
		f.bumpCalls = append(f.bumpCalls, id)
	}
	return nil
}

func (f *fakeUserRepo) UpdateSettings(_ context.Context, _ int64, mutate func(*UserSettings) error) (*UserSettings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := UserSettings{FavoriteMenus: append([]string(nil), f.settings.FavoriteMenus...)}
	if err := mutate(&s); err != nil {
		return nil, err
	}
	f.settings = s
	return &s, nil
}

// fakePermCache is an in-memory PermCache fake keyed by roleID.
type fakePermCache struct {
	data        map[int64][]string
	getCalls    map[int64]int
	setCalls    map[int64]int
	invalidated []int64
}

var _ PermCache = (*fakePermCache)(nil)

func (c *fakePermCache) GetRoleCodes(_ context.Context, roleID int64) ([]string, bool) {
	if c.getCalls == nil {
		c.getCalls = map[int64]int{}
	}
	c.getCalls[roleID]++
	codes, ok := c.data[roleID]
	return codes, ok
}

func (c *fakePermCache) SetRoleCodes(_ context.Context, roleID int64, codes []string) {
	if c.data == nil {
		c.data = map[int64][]string{}
	}
	if c.setCalls == nil {
		c.setCalls = map[int64]int{}
	}
	c.setCalls[roleID]++
	c.data[roleID] = codes
}

func (c *fakePermCache) InvalidateRole(_ context.Context, roleID int64) {
	c.invalidated = append(c.invalidated, roleID)
}

// noopLoginLog is a LoginLogger fake that discards everything.
type noopLoginLog struct{}

func (noopLoginLog) Log(context.Context, LoginEntry) {}
func (noopLoginLog) List(context.Context, LoginFilter, int, int) ([]*LoginRecord, int64, error) {
	return nil, 0, nil
}

func TestResolveAggregatesRoleCodes(t *testing.T) {
	repo := &fakeUserRepo{user: &User{ID: 7, Status: 1}, roles: []int64{1, 2}, roleCodes: map[int64][]string{1: {"sys:dept:view"}, 2: {"sys:account:view"}}}
	cache := &fakePermCache{}
	uc := NewAuthUseCase(repo, noopLoginLog{}, cache, nil, nil, time.Hour, nil, AuthConfig{}, nil)
	p, err := uc.Resolve(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Perms.Has("sys:dept:view") || !p.Perms.Has("sys:account:view") {
		t.Fatal("codes from all roles should be aggregated")
	}
}

func TestResolveRejectsDisabledUser(t *testing.T) {
	repo := &fakeUserRepo{user: &User{ID: 7, Status: 0}}
	uc := NewAuthUseCase(repo, noopLoginLog{}, &fakePermCache{}, nil, nil, time.Hour, nil, AuthConfig{}, nil)
	if _, err := uc.Resolve(context.Background(), 7); err == nil {
		t.Fatal("Resolve should reject a disabled user")
	}
}

// recordingLoginLog records the filter and count List received.
type recordingLoginLog struct {
	noopLoginLog
	filter LoginFilter
	limit  int
}

func (r *recordingLoginLog) List(_ context.Context, f LoginFilter, _ int, limit int) ([]*LoginRecord, int64, error) {
	r.filter, r.limit = f, limit
	return nil, 0, nil
}

// TestRecentLoginsOnlyOwn: the profile page's recent logins are fetched only for the user's own account and
// only as many as requested; without the account condition, "any logged-in user can see the whole site's
// login log".
func TestRecentLoginsOnlyOwn(t *testing.T) {
	log := &recordingLoginLog{}
	uc := NewAuthUseCase(&fakeUserRepo{}, log, &fakePermCache{}, nil, nil, time.Hour, nil, AuthConfig{}, nil)
	if _, err := uc.RecentLogins(context.Background(), 7, 5); err != nil {
		t.Fatalf("RecentLogins: %v", err)
	}
	if log.filter != (LoginFilter{UserID: 7}) || log.limit != 5 {
		t.Errorf("filter=%+v limit=%d, want UserID=7 limit=5", log.filter, log.limit)
	}
}
