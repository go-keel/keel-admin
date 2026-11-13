// Package timefmt contains the admin API's time serialization contract.
package timefmt

import (
	"errors"
	"time"
)

// RFC3339 returns an explicit UTC RFC3339 timestamp for frontend transport.
func RFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ParseRFC3339 parses an instant from a request; an empty string gives the zero value (the caller treats it as
// "unbounded").
//
// It accepts only RFC3339 and deliberately rejects wall-clock strings like "2006-01-02 15:04:05": accepting both
// reopens the question "whose time zone does the string without one use", which is exactly what switching requests
// to RFC3339 settled.
func ParseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// ErrBadRange reports a range that failed to parse: a bad format, or both ends given and the start not before the
// end.
var ErrBadRange = errors.New("timefmt: bad range")

// ParseInstantRange parses an instant range [from, to): both ends are UTC RFC3339, and an empty end gives the zero
// value (the caller treats it as "unbounded"). How to pass it: ADR admin-0003.
func ParseInstantRange(from, to string) (time.Time, time.Time, error) {
	f, err := ParseRFC3339(from)
	if err != nil {
		return time.Time{}, time.Time{}, ErrBadRange
	}
	t, err := ParseRFC3339(to)
	if err != nil {
		return time.Time{}, time.Time{}, ErrBadRange
	}
	return checked(f, t)
}

// ParseDateRange converts a date range (YYYY-MM-DD, both ends inclusive) into an instant range [from, to) in loc:
// the start is 00:00 of the first day and the end is 00:00 of the day **after** the last day. It adds one calendar
// day rather than 24 hours, because a daylight saving transition day has 23 or 25 hours.
func ParseDateRange(from, to string, loc *time.Location) (time.Time, time.Time, error) {
	day := func(s string) (time.Time, error) {
		if s == "" {
			return time.Time{}, nil
		}
		d, err := time.ParseInLocation(time.DateOnly, s, loc)
		if err != nil {
			return time.Time{}, ErrBadRange
		}
		return d, nil
	}
	f, err := day(from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	t, err := day(to)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !t.IsZero() {
		t = t.AddDate(0, 0, 1)
	}
	return checked(f, t)
}

func checked(from, to time.Time) (time.Time, time.Time, error) {
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return time.Time{}, time.Time{}, ErrBadRange
	}
	return from, to, nil
}
