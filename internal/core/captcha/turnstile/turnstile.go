// Package turnstile implements captcha.Provider on top of Cloudflare
// Turnstile: the widget renders in the browser with the site key, and this
// provider asks Cloudflare's siteverify endpoint whether the token it
// produced is genuine.
package turnstile

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-resty/resty/v2"

	"github.com/go-keel/keel-admin/internal/core/captcha"
	"github.com/go-keel/keel/core/khttpclient"
)

// Name is this provider's name in the registry and the value to set in sys.captcha.provider. It is exported so
// di.go and the config reading do not each write the literal: if the two drift apart, login silently fails closed.
const Name = "turnstile"

const (
	defaultEndpoint = "https://challenges.cloudflare.com"
	verifyPath      = "/turnstile/v0/siteverify"
)

// provider verifies Turnstile tokens against siteverify.
type provider struct {
	cli    *resty.Client
	secret string
}

var _ captcha.Provider = (*provider)(nil)

// New builds the Turnstile provider; cleanup closes idle connections.
//
// An empty secret is not an error here; that is Ready's job: an implementation that is not selected as the active
// provider must not break startup over missing config (as with base64, registration is unconditional and only the
// selected one is checked).
func New(secret string) (captcha.Provider, func(), error) {
	return newProvider(defaultEndpoint, secret)
}

func newProvider(endpoint, secret string) (*provider, func(), error) {
	cli, cleanup, err := khttpclient.New(endpoint,
		khttpclient.WithServiceName("cloudflare-turnstile"))
	if err != nil {
		return nil, nil, err
	}
	return &provider{cli: cli, secret: secret}, cleanup, nil
}

// Issue returns an empty challenge: the frontend renders the widget itself, so the server has nothing to issue.
func (p *provider) Issue(context.Context) (*captcha.Challenge, error) {
	return nil, nil
}

// siteverifyReply takes only the verdict and the error codes: this service does not check the other fields
// (challenge_ts, hostname, action), so nobody would read them.
type siteverifyReply struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify checks the token produced by the widget against siteverify. id is always empty (Issue issues no
// challenge), and answer is the token.
//
// remoteIP is sent along because Cloudflare uses it to bind the token to its source. It comes from
// metadata.GetRemoteIP, which trusts only the connection peer and the XFF written by listed proxies, so this
// provider, like the login IP allowlist, assumes nginx overwrites XFF as deploy-nginx.md describes (see AGENTS.md).
//
// Network errors and non-2xx responses return an error rather than (false, nil): the caller (step 0 of login in
// biz) fails closed on both, but folding "the check failed" and "the check never happened" into one value would
// make it impossible to tell from the log whether Cloudflare was unreachable or someone was guessing codes.
func (p *provider) Verify(ctx context.Context, _, answer, remoteIP string) (bool, error) {
	var out siteverifyReply
	resp, err := p.cli.R().
		SetContext(ctx).
		SetFormData(map[string]string{
			"secret":   p.secret,
			"response": answer,
			"remoteip": remoteIP,
		}).
		SetResult(&out).
		Post(verifyPath)
	if err != nil {
		return false, err
	}
	if resp.IsError() {
		return false, fmt.Errorf("turnstile: siteverify returned %s", resp.Status())
	}
	if !out.Success {
		return false, nil
	}
	return true, nil
}

// Ready requires the secret: selecting turnstile without a secret would silently turn the captcha off.
func (p *provider) Ready() error {
	if p.secret == "" {
		return errors.New("sys.captcha.turnstile.secret_key is required (never committed; injected by the deployment)")
	}
	return nil
}
