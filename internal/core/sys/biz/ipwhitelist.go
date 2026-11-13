package biz

import (
	"net/netip"
	"strings"
)

// MaxIPWhitelistLen is the length limit of the raw allowlist text, matching VARCHAR(512) of sys_user.ip_whitelist:
// a longer value would be truncated silently by the database, quietly loosening or tightening the rule.
const MaxIPWhitelistLen = 512

// parseIPWhitelist parses the comma-separated raw allowlist into a set of entries. An empty or all-blank string
// returns nil, meaning "no restriction". One invalid entry rejects the whole list: the allowlist is a security
// boundary, and tolerating a bad entry would let one typo quietly admit the whole world.
func parseIPWhitelist(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// Two forms are allowed: a single IP (1.2.3.4) and a CIDR (10.0.0.0/8).
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil {
				return nil, ErrInvalidArgument
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(item)
		if err != nil {
			return nil, ErrInvalidArgument
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// ValidateIPWhitelist checks the raw allowlist an administrator submits; the empty string is valid (no restriction).
func ValidateIPWhitelist(raw string) error {
	if len(raw) > MaxIPWhitelistLen {
		return ErrInvalidArgument
	}
	_, err := parseIPWhitelist(raw)
	return err
}

// ipAllowed reports whether the client IP falls within the allowlist.
//
// An empty allowlist means no restriction (allow). It is the default and the state of most accounts.
//
// It fails closed: when the allowlist is not empty but ip does not parse (the client address is unknown), it
// denies. An administrator set a restriction explicitly, so "we do not know where you come from" must not pass.
// Likewise, when the stored allowlist is bad data, parseIPWhitelist returns an error and callers treat it as a
// denial.
func ipAllowed(raw, ip string) bool {
	prefixes, err := parseIPWhitelist(raw)
	if err != nil {
		return false
	}
	if len(prefixes) == 0 {
		return true // no restriction set
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	// Unmap so that an IPv4-mapped IPv6 client (::ffff:1.2.3.4) matches a 1.2.3.4 rule.
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
