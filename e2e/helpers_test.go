//go:build e2e

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

// cookieName is server.http.default.auth.cookie_name in configs/config.yaml, read by TestE2E.
var cookieName string

// response is one HTTP exchange.
type response struct {
	code   int
	header http.Header
	body   []byte
}

// json decodes the body as a JSON object.
func (r response) json(t testing.TB) obj {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("response is not a JSON object (%v): %d %s", err, r.code, r.body)
	}
	return m
}

// ok fails the test unless the status is 200 and returns the body as an object.
func (r response) ok(t testing.TB) obj {
	t.Helper()
	return r.want(t, 200).json(t)
}

// want fails the test unless the status is code.
func (r response) want(t testing.TB, code int) response {
	t.Helper()
	if r.code != code {
		t.Fatalf("status %d, want %d: %s", r.code, code, r.body)
	}
	return r
}

// wantError fails the test unless the response is the structured error code + reason; it returns the message.
func (r response) wantError(t testing.TB, code int, reason string) string {
	t.Helper()
	m := r.want(t, code).json(t)
	if m["reason"] != reason {
		t.Fatalf("reason %v, want %s: %s", m["reason"], reason, r.body)
	}
	s, _ := m["message"].(string)
	return s
}

// session returns the value of the session cookie the response sets ("" when it sets none).
func (r response) session() (string, bool) {
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == cookieName {
			return c.Value, true
		}
	}
	return "", false
}

// setCookie returns the raw Set-Cookie header of the session cookie.
func (r response) setCookie() string {
	for _, v := range r.header.Values("Set-Cookie") {
		if strings.HasPrefix(v, cookieName+"=") {
			return v
		}
	}
	return ""
}

// obj is a decoded JSON object with typed getters; int64 fields arrive as strings (protojson).
type obj map[string]any

func (o obj) str(k string) string { s, _ := o[k].(string); return s }

func (o obj) id(k string) int64 {
	switch v := o[k].(type) {
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	case float64:
		return int64(v)
	}
	return 0
}

func (o obj) num(k string) float64 { n, _ := o[k].(float64); return n }

func (o obj) boolean(k string) bool { b, _ := o[k].(bool); return b }

func (o obj) list(k string) []obj {
	raw, _ := o[k].([]any)
	out := make([]obj, 0, len(raw))
	for _, v := range raw {
		m, _ := v.(map[string]any)
		out = append(out, m)
	}
	return out
}

func (o obj) strs(k string) []string {
	raw, _ := o[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

func (o obj) items() []obj { return o.list("items") }

func (o obj) total() int64 { return o.id("total") }

// col returns field k of every item.
func col(items []obj, k string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it[k]))
	}
	return out
}

// ids returns the id of every item.
func ids(items []obj) []int64 {
	out := make([]int64, 0, len(items))
	for _, it := range items {
		out = append(out, it.id("id"))
	}
	return out
}

// user is one account and its own cookie jar: the session cookie it holds, kept by hand because the committed
// configs set cookie_secure and the tests talk plain HTTP.
type user struct {
	e        *env
	name     string
	password string
	id       int64
	secret   string // authenticator secret once bound
	token    string // session cookie
	lang     string // Accept-Language; zh-CN when empty
	base     string // base URL of another admin process; the shared one when empty
}

// as returns a second jar of the same account (another browser): same credentials, no session.
func (u *user) as() *user {
	c := *u
	c.token = ""
	return &c
}

// at returns the same account, without a session, talking to the admin at base.
func (u *user) at(base string) *user {
	c := u.as()
	c.base = base
	return c
}

// do sends one request as u; body is marshalled to JSON unless it is a string; headers are name, value pairs.
func (u *user) do(t testing.TB, method, path string, body any, headers ...string) response {
	t.Helper()
	base := u.base
	if base == "" {
		base = u.e.base
	}
	r := send(t, base, method, path, body, u.token, u.lang, headers...)
	if v, ok := r.session(); ok {
		u.token = v
	}
	return r
}

func (u *user) get(t testing.TB, path string, headers ...string) response {
	t.Helper()
	return u.do(t, http.MethodGet, path, nil, headers...)
}

func (u *user) post(t testing.TB, path string, body any, headers ...string) response {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	return u.do(t, http.MethodPost, path, body, headers...)
}

func (u *user) put(t testing.TB, path string, body any, headers ...string) response {
	t.Helper()
	return u.do(t, http.MethodPut, path, body, headers...)
}

func (u *user) del(t testing.TB, path string, headers ...string) response {
	t.Helper()
	return u.do(t, http.MethodDelete, path, nil, headers...)
}

// login logs u in with its password and, when bound, a fresh code; headers go on the request.
func (u *user) login(t testing.TB, headers ...string) response {
	t.Helper()
	code := ""
	if u.secret != "" {
		code = u.code(t)
	}
	return u.loginWith(t, u.password, code, headers...)
}

func (u *user) loginWith(t testing.TB, password, code string, headers ...string) response {
	t.Helper()
	return u.post(t, "/v1/sys/auth/login", map[string]any{
		"username": u.name, "password": password, "otpCode": code, "remember": true,
	}, headers...)
}

// mustLogin logs u in and fails the test unless it gets a session.
func (u *user) mustLogin(t testing.TB) {
	t.Helper()
	u.token = ""
	u.login(t).want(t, 200)
	if u.token == "" {
		t.Fatalf("login %s: no session cookie", u.name)
	}
}

// code returns the current authenticator code of u with a fresh use quota: the replay counter of that code (Redis key
// otp:used:<user id>:<code hash>, internal/core/sys/data/otp_replay.go) is deleted first, so the tests are not held
// to sys.otp.max_uses uses per 30 seconds. Tests of the quota itself use rawCode.
func (u *user) code(t testing.TB) string {
	t.Helper()
	c := u.rawCode(t)
	sum := sha256.Sum256([]byte(c))
	key := fmt.Sprintf("otp:used:%d:%s", u.id, hex.EncodeToString(sum[:])[:16])
	if err := u.e.rdb.Del(context.Background(), key).Err(); err != nil {
		t.Fatal(err)
	}
	return c
}

// rawCode computes the current code from the secret as an authenticator app does (SHA1, 30 seconds, 6 digits;
// internal/core/sys/totp).
func (u *user) rawCode(t testing.TB) string {
	t.Helper()
	if u.secret == "" {
		t.Fatalf("%s has no authenticator bound", u.name)
	}
	c, err := totp.GenerateCode(u.secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// otp returns the X-OTP-Code header with a fresh code of u.
func (u *user) otp(t testing.TB) []string {
	t.Helper()
	return []string{"X-OTP-Code", u.code(t)}
}

// bind binds an authenticator through the bearer token (the restricted bind token, or "" for the session) and
// confirms it; it returns the confirm response.
func (u *user) bind(t testing.TB, bearer string, remember bool) response {
	t.Helper()
	var h []string
	if bearer != "" {
		h = []string{"Authorization", "Bearer " + bearer}
	}
	uri := u.post(t, "/v1/sys/auth/otp/bind", nil, h...).ok(t).str("provisioningUri")
	u.secret = secretOf(t, uri)
	return u.post(t, "/v1/sys/auth/otp/confirm", map[string]any{"code": u.code(t), "remember": remember}, h...)
}

// secretOf extracts the secret from an otpauth:// provisioning URI.
func secretOf(t testing.TB, uri string) string {
	t.Helper()
	p, err := url.Parse(uri)
	if err != nil || p.Scheme != "otpauth" || p.Query().Get("secret") == "" {
		t.Fatalf("not an otpauth:// URI with a secret: %q", uri)
	}
	return p.Query().Get("secret")
}

// send sends one request to base+path with the session cookie token.
func send(t testing.TB, base, method, path string, body any, token, lang string, headers ...string) response {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		rd = strings.NewReader(mustJSON(b))
	}
	req, err := http.NewRequest(method, base+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if lang == "" {
		lang = "zh-CN"
	}
	req.Header.Set("Accept-Language", lang)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Add(headers[i], headers[i+1])
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{code: resp.StatusCode, header: resp.Header, body: b}
}

var noRedirect = &http.Client{
	Timeout:       30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

var seq atomic.Int64

// uniq returns prefix with a suffix unique within the test run, made of letters and digits only (no _ or %, which
// the keyword tests give a meaning).
func uniq(prefix string) string {
	return fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano()%1e6, seq.Add(1))
}

// xff returns an X-Forwarded-For header with a client IP unique within the run: the server trusts only localhost, so
// the header is the client IP (keel:docs/metadata.md, "How the client IP is computed").
func xff() []string {
	n := seq.Add(1)
	return []string{"X-Forwarded-For", fmt.Sprintf("10.%d.%d.%d", 1+n/65536%250, n/256%256, n%256)}
}

// with joins header pairs.
func with(hs ...[]string) []string { return slices.Concat(hs...) }

// q builds a query string from name, value pairs.
func q(kv ...any) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Add(fmt.Sprint(kv[i]), fmt.Sprint(kv[i+1]))
	}
	return "?" + v.Encode()
}

// path joins parts with "/".
func path(parts ...any) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = fmt.Sprint(p)
	}
	return strings.Join(s, "/")
}

// has reports whether list contains v.
func has[T comparable](list []T, v T) bool { return slices.Contains(list, v) }

// mustHave and mustLack fail the test when list lacks or holds v.
func mustHave[T comparable](t testing.TB, what string, list []T, v T) {
	t.Helper()
	if !slices.Contains(list, v) {
		t.Fatalf("%s: %v lacks %v", what, list, v)
	}
}

func mustLack[T comparable](t testing.TB, what string, list []T, v T) {
	t.Helper()
	if slices.Contains(list, v) {
		t.Fatalf("%s: %v holds %v", what, list, v)
	}
}

// eq fails the test unless got equals want.
func eq[T comparable](t testing.TB, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}
