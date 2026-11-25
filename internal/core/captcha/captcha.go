// Package captcha declares the Provider kernel contract: the interface a captcha implementation must satisfy.
//
// Two providers ship in-tree, base64 (an image the server draws) and turnstile (Cloudflare's widget);
// server.NewKernel builds the one sys.captcha.provider names, or none when captcha is off. Only these two are
// supported: the login page renders only them.
//
// Zero dependencies beyond the standard library, so the interface is importable from internal/core/sys/biz without a
// dependency cycle.
package captcha

import "context"

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
