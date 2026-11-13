package data

import "testing"

// A keyword is matched literally: its LIKE wildcards and the escape character are escaped.
func TestContainsPatternEscapesWildcards(t *testing.T) {
	cases := map[string]string{
		"alice": `%alice%`,
		"u_":    `%u\_%`,
		"50%":   `%50\%%`,
		`a\b`:   `%a\\b%`,
	}
	for in, want := range cases {
		if got := containsPattern(in); got != want {
			t.Errorf("containsPattern(%q) = %q, want %q", in, got, want)
		}
	}
}
