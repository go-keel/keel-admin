package timefmt

import (
	"testing"
	"time"
)

func TestRFC3339NormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+8", 8*60*60)
	got := RFC3339(time.Date(2026, 7, 25, 18, 30, 45, 0, loc))
	if got != "2026-07-25T10:30:45Z" {
		t.Fatalf("RFC3339() = %q, want UTC timestamp", got)
	}
}

func TestRFC3339ZeroIsEmpty(t *testing.T) {
	if got := RFC3339(time.Time{}); got != "" {
		t.Fatalf("RFC3339(zero) = %q, want empty", got)
	}
}

func TestParseRFC3339RoundTrips(t *testing.T) {
	at, err := ParseRFC3339("2026-07-25T10:30:45Z")
	if err != nil {
		t.Fatalf("ParseRFC3339() error = %v", err)
	}
	if got := RFC3339(at); got != "2026-07-25T10:30:45Z" {
		t.Fatalf("round trip = %q", got)
	}
}

func TestParseRFC3339EmptyIsZero(t *testing.T) {
	at, err := ParseRFC3339("")
	if err != nil || !at.IsZero() {
		t.Fatalf("ParseRFC3339(\"\") = %v, %v; want zero, nil", at, err)
	}
}

// Wall-clock strings are rejected: accepting both formats reopens "whose time zone does the string without one use".
func TestParseRFC3339RejectsWallClock(t *testing.T) {
	if _, err := ParseRFC3339("2026-07-25 10:30:45"); err == nil {
		t.Fatal("ParseRFC3339(wall clock) = nil error, want error")
	}
}

func TestParseInstantRange(t *testing.T) {
	from, to, err := ParseInstantRange("2026-09-01T00:00:00Z", "")
	if err != nil || !to.IsZero() || RFC3339(from) != "2026-09-01T00:00:00Z" {
		t.Fatalf("open end: %v %v %v", from, to, err)
	}
	if _, _, err := ParseInstantRange("2026-09-01", ""); err == nil {
		t.Fatal("date-only accepted as instant")
	}
	// Half-open: equal start and end is an empty range, rejected instead of silently returning zero rows.
	if _, _, err := ParseInstantRange("2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z"); err == nil {
		t.Fatal("empty range accepted")
	}
}

func TestParseDateRangeIncludesBothEnds(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	from, to, err := ParseDateRange("2026-09-01", "2026-09-01", sh)
	if err != nil {
		t.Fatal(err)
	}
	if RFC3339(from) != "2026-08-31T16:00:00Z" || RFC3339(to) != "2026-09-01T16:00:00Z" {
		t.Fatalf("single day = [%s, %s)", RFC3339(from), RFC3339(to))
	}
	if _, _, err := ParseDateRange("2026-09-02", "2026-09-01", sh); err == nil {
		t.Fatal("reversed range accepted")
	}
	if _, _, err := ParseDateRange("2026-09-01T00:00:00Z", "", sh); err == nil {
		t.Fatal("instant accepted as date")
	}
}

// New York springs forward on 2026-03-08, a day of only 23 hours: the end adds one calendar day, not 24 hours.
func TestParseDateRangeDSTDay(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	from, to, err := ParseDateRange("2026-03-08", "2026-03-08", ny)
	if err != nil {
		t.Fatal(err)
	}
	if got := to.Sub(from); got != 23*time.Hour {
		t.Fatalf("DST day length = %v", got)
	}
}
