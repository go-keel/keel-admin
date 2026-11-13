package biz

import (
	"encoding/base64"
	"strings"
)

// MaxAvatarBytes is the limit on the decoded avatar (256 KiB). The avatar is sent with every Profile and takes up
// row width in the database, so the limit is tighter than "whatever uploads"; the frontend already scales images
// down to 256px square, so a normal avatar is far smaller.
const MaxAvatarBytes = 256 << 10

// avatarPrefixes is the allowlist of data URI prefixes. Only bitmaps, base64-encoded: browsers render SVG as a
// document, so an inline script becomes stored XSS, and the avatar is echoed on every page that shows the user, an
// ideal delivery surface. image/svg+xml is therefore never accepted.
var avatarPrefixes = []string{
	"data:image/png;base64,",
	"data:image/jpeg;base64,",
	"data:image/webp;base64,",
}

// validateAvatar checks an avatar data URI; the empty string is valid and clears the avatar. It returns
// ErrInvalidArgument rather than a finer error: a bad format and an oversize image are the same "this image cannot
// be used" to the caller, and a finer error would not help fix it.
func validateAvatar(avatar string) error {
	if avatar == "" {
		return nil
	}
	payload := ""
	for _, p := range avatarPrefixes {
		if strings.HasPrefix(avatar, p) {
			payload = avatar[len(p):]
			break
		}
	}
	if payload == "" {
		return ErrInvalidArgument
	}
	// Judge by the decoded size: base64 inflates the data by about 4/3, so measuring the string misjudges it.
	// DecodeString also verifies that the payload is valid base64 rather than arbitrary bytes.
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return ErrInvalidArgument
	}
	if len(raw) > MaxAvatarBytes {
		return ErrAvatarTooLarge
	}
	return nil
}
