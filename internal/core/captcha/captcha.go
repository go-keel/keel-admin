// Package captcha declares the CaptchaProvider kernel contract: the
// interface a captcha implementation must satisfy, plus a small
// name -> Provider registry so sys.captcha.provider can select one by name.
//
// Only the base64 image provider ships in-tree. The interface + registry are
// the extension point: implement Provider, MustRegister it in di.go, and
// point sys.captcha.provider at its name — nothing else changes.
//
// Zero business dependencies: stdlib, context and the kernel's rule namespace only. This keeps the
// interface importable from internal/core/sys/biz without creating a
// dependency cycle.
package captcha

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-keel/keel-admin/internal/core/errs"
)

// Challenge is one issued captcha challenge. ImageBase64 is empty for
// providers that render nothing server-side (e.g. a client-rendered widget
// verified against a remote service).
type Challenge struct {
	ID          string
	ImageBase64 string
}

// Provider is the kernel contract every captcha implementation satisfies.
type Provider interface {
	// Issue creates a new challenge. A provider that needs no server-issued
	// challenge may return (nil, nil).
	Issue(ctx context.Context) (*Challenge, error)
	// Verify checks a challenge answer. remoteIP is passed through for
	// providers that include it in their upstream verification call.
	Verify(ctx context.Context, id, answer, remoteIP string) (bool, error)
	// Ready self-checks that the provider is usable right now (e.g. its
	// secret is configured). A non-nil error means this provider must not
	// be selected as the active sys.captcha.provider.
	Ready() error
}

// Registry: simpler than perm's — no Seal, since the provider count is
// small and fixed by imported packages (di.go), not a per-request concern.
// Duplicate registration is a programming error and panics.
var (
	mu       sync.Mutex
	registry = map[string]Provider{}
)

// MustRegister registers a named captcha provider. An empty name or a
// duplicate name panics — both are assembly-time (di.go/init) programming
// errors.
func MustRegister(name string, p Provider) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic(errs.Rules.New("captcha.provider-name-empty", "captcha.MustRegister was called with an empty provider name",
			"pass the name sys.captcha.provider will use"))
	}
	if _, dup := registry[name]; dup {
		panic(errs.Rules.New("captcha.provider-duplicate", fmt.Sprintf("captcha provider %s is registered twice", name),
			"register each provider once in di.go"))
	}
	registry[name] = p
}

// Get returns the registered provider by name, if any.
func Get(name string) (Provider, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := registry[name]
	return p, ok
}

// Names returns the registered provider names (unordered).
func Names() []string {
	mu.Lock()
	defer mu.Unlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}

// ResetForTest is a test-only hook to clear the registry between test
// cases. Exported (unlike perm/config's unexported resetForTest) because
// this registry's consumers span packages: sys/biz's SetConfig validation
// needs to seed and tear down fake providers in the global registry from
// its own test package.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Provider{}
}
