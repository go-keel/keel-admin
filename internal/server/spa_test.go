package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	webui "github.com/go-keel/keel-admin/web"
)

func TestLogoHandlerServesOneEntryForEveryMode(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")

	t.Run("embedded asset", func(t *testing.T) {
		h := newLogoHandler("", "", "logo.png", png)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		// The endpoint has no extension, yet Content-Type follows the embedded file name: the URL is independent of
		// the format, so the endpoint needs no second path like /logo.png
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Fatalf("Content-Type = %q, want image/png", ct)
		}
		if rec.Header().Get("Etag") == "" {
			t.Fatal("embedded asset must carry an ETag so repeat loads go 304")
		}
		if !bytes.Equal(rec.Body.Bytes(), png) {
			t.Fatal("body is not the embedded asset")
		}
	})

	t.Run("fork ships svg", func(t *testing.T) {
		// Assembly seam: a derived repo replaces public/logo.png with logo.svg, and the endpoint URL stays the same
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
		h := newLogoHandler("", "", "logo.svg", svg)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

		if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Fatalf("Content-Type = %q, want image/svg+xml", ct)
		}
	})

	t.Run("disk logo wins over the embedded one", func(t *testing.T) {
		dir := t.TempDir()
		disk := filepath.Join(dir, "brand.svg")
		svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)
		if err := os.WriteFile(disk, svg, 0o600); err != nil {
			t.Fatal(err)
		}

		h := newLogoHandler("", disk, "logo.png", png)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

		if !bytes.Equal(rec.Body.Bytes(), svg) {
			t.Fatal("body is not the disk logo")
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
			t.Fatalf("Content-Type = %q, want image/svg+xml", ct)
		}
	})

	t.Run("external url gets a redirect, not a proxy", func(t *testing.T) {
		h := newLogoHandler("https://cdn.example.com/brand.svg", "", "logo.png", png)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
		if got := rec.Header().Get("Location"); got != "https://cdn.example.com/brand.svg" {
			t.Fatalf("Location = %q, want the external url", got)
		}
		// The server does not proxy external content: the response body must not contain the asset's bytes
		if rec.Body.Len() > 0 && bytes.Contains(rec.Body.Bytes(), png) {
			t.Fatal("external logo must not be proxied through the server")
		}
	})

	t.Run("external url wins over a disk logo", func(t *testing.T) {
		h := newLogoHandler("https://cdn.example.com/brand.svg", "/etc/keel/brand.png", "logo.png", png)
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302", rec.Code)
		}
	})
}

func TestEmbeddedLogoServesThroughTheEntry(t *testing.T) {
	// The real embedded asset goes through the whole endpoint: the embed in web/logo.go → endpoint → content.
	// It does not depend on the embed_ui tag, so go run in development and the embedded deployment take the same path.
	name, data := webui.Logo()
	if len(data) == 0 {
		t.Fatal("web.Logo() returned no data — public/logo.* missing from the binary")
	}

	h := newLogoHandler("", "", name, data)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, logoEntry, nil))

	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("entry did not serve the embedded asset (status %d)", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Fatalf("Content-Type = %q, want an image type", ct)
	}
	t.Logf("embedded logo %s → %s (%s, %d bytes)", name, logoEntry,
		rec.Header().Get("Content-Type"), len(data))
}

func TestBrandIndexInjectsTitleAndLogoEntry(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html": {
			Data: []byte("<html><head><title>Old</title></head><body></body></html>"),
		},
	}

	got := string(brandIndex(webFS, "Brand"))

	// The favicon and the in-page logo share one endpoint, with no hard-coded format
	if !strings.Contains(got, `<link rel="icon" href="/v1/sys/brand/logo">`) {
		t.Fatalf("index does not point favicon at the logo entry: %s", got)
	}
	if !strings.Contains(got, "<title>Brand</title>") {
		t.Fatalf("index title not replaced: %s", got)
	}
	// The logo is not sent through __BRAND__: the frontend always uses the endpoint URL and need not know its target
	if strings.Contains(got, `"logo"`) {
		t.Fatalf("index should not carry a logo address: %s", got)
	}
}
