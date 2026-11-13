package biz

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func dataURI(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// An empty string clears the avatar and is valid.
func TestValidateAvatarEmptyIsAllowed(t *testing.T) {
	if err := validateAvatar(""); err != nil {
		t.Fatalf("an empty avatar should be valid (it clears): %v", err)
	}
}

func TestValidateAvatarAcceptsBitmapTypes(t *testing.T) {
	for _, mime := range []string{"image/png", "image/jpeg", "image/webp"} {
		if err := validateAvatar(dataURI(mime, []byte("small"))); err != nil {
			t.Fatalf("%s should be accepted: %v", mime, err)
		}
	}
}

// The browser renders SVG as a document, so an inline script is stored XSS; the avatar is echoed on every page
// that can see the user, an ideal delivery surface. It must be rejected.
func TestValidateAvatarRejectsSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if err := validateAvatar(dataURI("image/svg+xml", svg)); err == nil {
		t.Fatal("an SVG avatar must be rejected: an inline script is stored XSS")
	}
}

// Anything that is not a data URI (external links, bare base64, javascript: and so on) is rejected.
func TestValidateAvatarRejectsNonDataURI(t *testing.T) {
	for _, bad := range []string{
		"https://evil.example/a.png",
		"javascript:alert(1)",
		base64.StdEncoding.EncodeToString([]byte("naked")),
		"data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<h1>x")),
	} {
		if err := validateAvatar(bad); err == nil {
			t.Fatalf("an invalid avatar should be rejected: %q", bad)
		}
	}
}

// Claims to be png but the payload is not valid base64: rejected.
func TestValidateAvatarRejectsBadBase64(t *testing.T) {
	if err := validateAvatar("data:image/png;base64,!!!not-base64!!!"); err == nil {
		t.Fatal("an invalid base64 payload should be rejected")
	}
}

// The size is judged after decoding: base64 inflates by about 4/3, so the raw string length would misjudge.
func TestValidateAvatarSizeLimitIsOnDecodedBytes(t *testing.T) {
	ok := dataURI("image/png", make([]byte, MaxAvatarBytes))
	if err := validateAvatar(ok); err != nil {
		t.Fatalf("exactly at the limit should pass: %v", err)
	}
	// This string's length already exceeds the limit (4/3 inflation), but decoded it is within the limit: it must not
	// be rejected for that.
	if len(ok) <= MaxAvatarBytes {
		t.Fatal("broken test case: the encoded length must exceed the limit to show the decoded size is what counts")
	}

	tooBig := dataURI("image/png", make([]byte, MaxAvatarBytes+1))
	if err := validateAvatar(tooBig); !errors.Is(err, ErrAvatarTooLarge) {
		t.Fatalf("over the limit should return ErrAvatarTooLarge, got %v", err)
	}
}

// The prefix whitelist is an exact match and cannot be bypassed by a disguised prefix.
func TestValidateAvatarRejectsSpoofedPrefix(t *testing.T) {
	spoof := "data:image/png;base64x," + strings.Repeat("A", 8)
	if err := validateAvatar(spoof); err == nil {
		t.Fatal("a disguised prefix should be rejected")
	}
}
