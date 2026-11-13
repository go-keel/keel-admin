package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-keel/keel-admin/internal/conf"
	webui "github.com/go-keel/keel-admin/web"
	"github.com/go-keel/keel/server/api"
)

// Vite (rollup) names its output `<name>-<8-character hash>.js|css`, with the hash in the base64url alphabet
// (including - and _), not hexadecimal: matching hexadecimal would hit nothing.
// Fixed-name assets under public/ do not match: they have no hash, and a long cache would keep updates from taking
// effect.
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8}\.(js|css)$`)

var titleTagRe = regexp.MustCompile(`<title>[^<]*</title>`)

// The two brand paths live under /v1/sys/: raw handlers share the API namespace with proto RPCs, structurally apart
// from frontend routes (root level, owned by the SPA), and `/v1/sys/*` is the kernel's area that business endpoints
// do not use, so they cannot collide with anything in a derived repo. Why raw handlers go under /v1 too instead of a
// top-level prefix of their own: ADR admin-0012.
const (
	brandEntry = "/v1/sys/brand"
	// logoEntry is the one endpoint of the brand logo: all three states (embedded / disk / external URL) and both
	// forms (dev and embedded deployment) share it. Callers (in-page logo, favicon, external references) therefore
	// need not know where the logo is or what format it has: the server sets Content-Type from the real file,
	// independent of the URL.
	logoEntry = brandEntry + "/logo"
)

// RegisterSPA returns the streamRegister mounting the embedded React frontend
// (built with -tags embed_ui), with deploy-time branding applied. It also owns
// the two brand paths under /v1/sys/; every other /v1/* route belongs to the
// proto handlers, and all remaining paths fall back to index.html.
//
// Both brand paths are registered in both forms, so dev (go run without the embedded UI) behaves like the embedded
// deployment: the embedded logo fallback does not depend on embed_ui (see web/logo.go), and dev reaches the backend
// through the existing /v1 proxy in web/vite.config.ts, with no extra proxy for the brand.
func RegisterSPA(brand *conf.Brand) api.StreamRegister {
	return func(sc *api.StreamContext) {
		title := brand.GetTitle()
		// The logo setting is tri-state: empty = embedded default; http(s):// = external URL; anything else = disk path.
		var diskPath, logoURL string
		switch cfg := brand.GetLogo(); {
		case cfg == "":
		case strings.HasPrefix(cfg, "http://"), strings.HasPrefix(cfg, "https://"):
			logoURL = cfg
		default:
			if _, err := os.Stat(cfg); err != nil {
				// The brand is cosmetic, so a wrong path does not refuse startup; warn and fall back to the embedded logo
				slog.Warn("sys.brand.logo not readable, falling back to embedded logo",
					"path", cfg, "error", err)
			} else {
				diskPath = cfg
			}
		}

		// The machine-readable form of the brand. The embedded frontend does not need it (__BRAND__ is already
		// injected into index.html, see the __BUILD_ID__ guard in brand.ts), so in practice it serves only dev; it is
		// mounted in both modes so behavior does not fork and curl can inspect it. The logo is not sent here: it has
		// the logoEntry endpoint, and the frontend references that one URL in every state.
		brandJSON, _ := json.Marshal(map[string]string{"title": title})
		serveBrandJSON := func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(brandJSON)
		}

		// StreamRegister runs after the proto handlers and gorilla mux matches in registration order: these two
		// overlap no proto route and are registered before HandlePrefix("/") below, so both are reachable. All three
		// declare anonymous access explicitly with sc.Anonymous(): the login page needs the logo and the frontend
		// shell itself. A request with credentials still has its identity resolved as usual, so logged-in users are
		// unaffected.
		embeddedName, embeddedData := webui.Logo()
		sc.Anonymous().HandleFunc(brandEntry, serveBrandJSON)
		sc.Anonymous().HandleFunc(logoEntry, newLogoHandler(logoURL, diskPath, embeddedName, embeddedData))

		webFS := webui.FS()
		if webFS == nil {
			return
		}

		branded := brandIndex(webFS, title)
		fileServer := http.FileServer(http.FS(webFS))
		// HandlePrefix: gorilla mux's HandleFunc("/") matches "/" exactly, so deep links (/sys/users) and static files
		// (/version.json) would 404; only a prefix route catches everything.
		// Registered anonymously: the frontend shell (including the login page itself) must be reachable logged out.
		sc.Anonymous().HandlePrefix("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Registered /v1 routes (proto and the two brand paths above) match first; what lands here is an API path
			// nobody claims, so it returns 404 instead of index.html.
			if strings.HasPrefix(r.URL.Path, "/v1") {
				http.NotFound(w, r)
				return
			}
			if r.URL.Path != "/" {
				if _, err := fs.Stat(webFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
					r.URL.Path = "/" // SPA fallback
				}
			}
			// index.html and version.json are not cached (update detection depends on getting the latest); hash-named
			// output is immutable, marked so for long browser/CDN caching, and an update fetches only changed files
			if r.URL.Path == "/" || r.URL.Path == "/version.json" {
				w.Header().Set("Cache-Control", "no-store")
			} else if hashedAsset.MatchString(r.URL.Path) {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			if r.URL.Path == "/" && branded != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(branded)
				return
			}
			fileServer.ServeHTTP(w, r)
		}))
	}
}

// newLogoHandler serves the brand logo on its single entry.
// Three states, two behaviors: an external URL gets one 302 and the browser fetches the content from the CDN itself.
// Not proxying external URLs is a settled decision of sys.brand.logo: a proxy would tie the CDN's availability to
// this service. The embedded and disk states serve the content directly, with Content-Type inferred by
// ServeContent/ServeFile from the real file name, regardless of whether the request path has an extension, so the
// endpoint needs no second path with an extension.
// no-cache = revalidate before every use: in the disk state ServeFile answers 304 by mtime, so replacing the file's
// content in place takes effect immediately; the embedded state is fixed for a given binary and answers 304 by a
// content ETag.
func newLogoHandler(logoURL, diskPath, embeddedName string, embeddedData []byte) http.HandlerFunc {
	sum := sha256.Sum256(embeddedData)
	etag := fmt.Sprintf(`"%x"`, sum[:8])
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		if logoURL != "" {
			http.Redirect(w, r, logoURL, http.StatusFound)
			return
		}
		if diskPath != "" {
			http.ServeFile(w, r, diskPath)
			return
		}
		w.Header().Set("Etag", etag)
		http.ServeContent(w, r, embeddedName, time.Time{}, bytes.NewReader(embeddedData))
	}
}

// brandIndex applies the deploy-time brand to the embedded index.html:
// title → replaces the <title> built in and injects window.__BRAND__ for the SPA to read at runtime (header, login
// page, see web/src/lib/brand.ts); the favicon points at logoEntry, the same endpoint as the in-page logo. No type
// attribute: the endpoint's Content-Type decides the format.
func brandIndex(webFS fs.FS, title string) []byte {
	raw, err := fs.ReadFile(webFS, "index.html")
	if err != nil {
		return nil
	}
	s := string(raw)
	var inject strings.Builder
	if title != "" {
		// json.Marshal escapes <>& by default, so inlining into <script> cannot be cut short by "</script>"
		b, err := json.Marshal(map[string]string{"title": title})
		if err != nil {
			return nil
		}
		inject.WriteString("<script>window.__BRAND__=" + string(b) + "</script>")
		s = titleTagRe.ReplaceAllLiteralString(s,
			"<title>"+html.EscapeString(title)+"</title>")
	}
	inject.WriteString(`<link rel="icon" href="` + logoEntry + `">`)
	return []byte(strings.Replace(s, "<head>", "<head>"+inject.String(), 1))
}
