package server

import (
	"strings"
	"testing"

	"github.com/go-keel/keel-admin/internal/conf"
)

// newCaptchaProvider builds nothing when captcha is off, and fails startup on an unknown name or a provider that is
// not ready.
func TestNewCaptchaProvider(t *testing.T) {
	for _, c := range []struct {
		name    string
		cfg     *conf.Captcha
		wantNil bool
		wantErr string
	}{
		{"off", &conf.Captcha{Enabled: false, Provider: "nope"}, true, ""},
		{"base64", &conf.Captcha{Enabled: true, Provider: "base64"}, false, ""},
		{"unknown", &conf.Captcha{Enabled: true, Provider: "nope"}, true, "captcha.provider-unknown"},
		{"turnstile without secret", &conf.Captcha{Enabled: true, Provider: "turnstile"}, true, "captcha.provider-not-ready"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, cleanup, err := newCaptchaProvider(c.cfg, nil)
			cleanup()
			if c.wantErr == "" && err != nil || c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("err = %v, want %q", err, c.wantErr)
			}
			if (p == nil) != c.wantNil {
				t.Fatalf("provider = %v, want nil: %v", p, c.wantNil)
			}
		})
	}
}
