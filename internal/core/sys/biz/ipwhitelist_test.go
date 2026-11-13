package biz

import (
	"strings"
	"testing"
)

// An empty whitelist means no restriction (the default for almost every account).
func TestIPAllowedEmptyMeansUnrestricted(t *testing.T) {
	for _, raw := range []string{"", "   ", ",", " , "} {
		if !ipAllowed(raw, "1.2.3.4") {
			t.Fatalf("an empty whitelist should pass: %q", raw)
		}
	}
}

func TestIPAllowedSingleAddressAndCIDR(t *testing.T) {
	cases := []struct {
		raw, ip string
		want    bool
	}{
		{"1.2.3.4", "1.2.3.4", true},
		{"1.2.3.4", "1.2.3.5", false},
		{"10.0.0.0/8", "10.9.9.9", true},
		{"10.0.0.0/8", "11.0.0.1", false},
		{"1.2.3.4, 10.0.0.0/8", "10.1.1.1", true},
		{" 1.2.3.4 , 10.0.0.0/8 ", "1.2.3.4", true}, // whitespace around entries should be ignored
		{"::1", "::1", true},
		{"2001:db8::/32", "2001:db8::1", true},
		{"2001:db8::/32", "2001:dead::1", false},
	}
	for _, c := range cases {
		if got := ipAllowed(c.raw, c.ip); got != c.want {
			t.Fatalf("ipAllowed(%q, %q) = %v, want %v", c.raw, c.ip, got, c.want)
		}
	}
}

// Reverse proxies often pass IPv4 as IPv4-mapped IPv6; it must match IPv4 rules, or the rules look right but
// all fail.
func TestIPAllowedUnmapsIPv4MappedIPv6(t *testing.T) {
	if !ipAllowed("1.2.3.4", "::ffff:1.2.3.4") {
		t.Fatal("IPv4-mapped IPv6 should match the equivalent IPv4 rule")
	}
	if !ipAllowed("10.0.0.0/8", "::ffff:10.1.2.3") {
		t.Fatal("IPv4-mapped IPv6 should match the equivalent IPv4 CIDR")
	}
}

// Whitelist set but the source IP is unavailable: fail closed. Once an admin has set a restriction, "we do not
// know where you are from" must not pass.
func TestIPAllowedFailsClosedOnUnknownSource(t *testing.T) {
	if ipAllowed("1.2.3.4", "") {
		t.Fatal("with a restriction set, an unknown source IP must be rejected")
	}
	if ipAllowed("1.2.3.4", "not-an-ip") {
		t.Fatal("with a restriction set, an invalid source IP must be rejected")
	}
}

// Bad data stored in the whitelist (written to the database past validation, for example): reject entirely,
// never degrade to allowing.
func TestIPAllowedFailsClosedOnCorruptRule(t *testing.T) {
	if ipAllowed("1.2.3.4, garbage", "1.2.3.4") {
		t.Fatal("an invalid entry must reject everything; tolerating bad entries lets one typo admit the whole world")
	}
}

func TestValidateIPWhitelist(t *testing.T) {
	for _, ok := range []string{"", "1.2.3.4", "10.0.0.0/8", "1.2.3.4, 10.0.0.0/8", "::1"} {
		if err := ValidateIPWhitelist(ok); err != nil {
			t.Fatalf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"garbage", "1.2.3.4/99", "1.2.3.256", "1.2.3.4;10.0.0.1"} {
		if err := ValidateIPWhitelist(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
	// Anything too long would be silently truncated by VARCHAR(512), quietly changing the rule: it must be rejected
	// before it is stored.
	if err := ValidateIPWhitelist(strings.Repeat("1.2.3.4,", 200)); err == nil {
		t.Fatal("exceeding MaxIPWhitelistLen should be rejected (otherwise truncation on insert quietly rewrites the rule)")
	}
}
