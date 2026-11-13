package captcha

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
)

// fakeProvider is a minimal Provider stand-in whose Ready() outcome is
// controlled per test case.
type fakeProvider struct {
	readyErr error
}

var _ Provider = (*fakeProvider)(nil)

func (f *fakeProvider) Issue(_ context.Context) (*Challenge, error) {
	return &Challenge{ID: "id", ImageBase64: "img"}, nil
}

func (f *fakeProvider) Verify(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

func (f *fakeProvider) Ready() error {
	return f.readyErr
}

func TestMustRegisterAndGet(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	p := &fakeProvider{}
	MustRegister("fake", p)

	got, ok := Get("fake")
	if !ok {
		t.Fatal("Get: expected fake provider to be registered")
	}
	if got != Provider(p) {
		t.Fatalf("Get: got %v, want %v", got, p)
	}
}

func TestMustRegisterDuplicatePanics(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	MustRegister("dup", &fakeProvider{})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	MustRegister("dup", &fakeProvider{})
}

func TestMustRegisterEmptyNamePanics(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on empty name registration")
		}
	}()
	MustRegister("", &fakeProvider{})
}

func TestGetMiss(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	_, ok := Get("no-such-provider")
	if ok {
		t.Fatal("Get: expected miss for unregistered provider")
	}
}

func TestNames(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	MustRegister("b", &fakeProvider{})
	MustRegister("a", &fakeProvider{})

	names := Names()
	sort.Strings(names)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Names() = %v, want %v", names, want)
	}
}

func TestProviderReadyControllable(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	notReady := &fakeProvider{readyErr: errors.New("secret not configured")}
	MustRegister("not-ready", notReady)

	got, ok := Get("not-ready")
	if !ok {
		t.Fatal("Get: expected not-ready provider to be registered")
	}
	if got.Ready() == nil {
		t.Fatal("Ready: expected non-nil error")
	}
}
