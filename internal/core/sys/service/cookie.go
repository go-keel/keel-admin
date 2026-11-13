package service

import (
	"context"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v3/transport"
)

// setSessionCookie issues the session cookie on the response.
//
// The token travels in an httpOnly cookie instead of being stored by the frontend because what the frontend never
// touches, XSS cannot steal. The cost is that the browser sends the cookie automatically, which opens CSRF; that is
// shut by SameSite=Strict. The SPA and the API share an origin (a single binary), so Strict causes no cross-site
// trouble.
//
// persist means "remember me": true writes Max-Age (the cookie survives closing the browser), false writes a session
// cookie (no Max-Age, dropped when the browser closes). The token's own lifetime is the same either way
// (sys.session_ttl): "remember me" only decides whether to keep it on disk and must not change how long the server
// issues it for.
//
// secure comes from sys.cookie_secure and must be true in production, otherwise the cookie leaks with plain-text
// requests. It is config rather than TLS sniffing because behind a proxy r.TLS is always nil, and sniffing would
// reach the wrong conclusion and quietly turn Secure off.
func setSessionCookie(ctx context.Context, name, token string, ttl time.Duration, persist, secure bool) {
	c := &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	}
	if persist {
		c.MaxAge = int(ttl.Seconds())
	}
	writeCookie(ctx, c)
}

// clearSessionCookie makes the browser drop the session cookie at once (logout). The attributes must match those at
// issue, otherwise the browser treats it as a different cookie and does not overwrite it.
func clearSessionCookie(ctx context.Context, name string, secure bool) {
	writeCookie(ctx, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func writeCookie(ctx context.Context, c *http.Cookie) {
	if tr, ok := transport.FromServerContext(ctx); ok {
		tr.ReplyHeader().Add("Set-Cookie", c.String())
	}
}
