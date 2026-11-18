// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package httpdomain matches HTTP hostnames against operator-configured domain
// patterns: exact hostnames, IP addresses, or *.domain wildcards.
package httpdomain

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var errInvalidPattern = errors.New("invalid HTTP domain pattern")

// ValidatePatterns rejects URLs, ports, and ambiguous wildcard patterns.
func ValidatePatterns(patterns []string) error {
	for _, pattern := range patterns {
		if err := ValidatePattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePattern accepts a hostname, an IP address without zone, or a
// *.domain pattern.
func ValidatePattern(pattern string) error {
	host := strings.TrimSuffix(strings.ToLower(pattern), ".")
	wildcard := strings.HasPrefix(host, "*.")
	host = strings.TrimPrefix(host, "*.")
	if ip, err := netip.ParseAddr(host); err == nil {
		if wildcard || ip.Zone() != "" {
			return errInvalidPattern
		}
		return nil
	}
	if len(host) == 0 || len(host) > 253 {
		return errInvalidPattern
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errInvalidPattern
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return errInvalidPattern
			}
		}
	}
	return nil
}

// AllowsURL compares parsed hostnames, never URL prefixes or DNS results.
func AllowsURL(patterns []string, u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.Hostname() == "" {
		return false
	}
	return AllowsHost(patterns, u.Hostname())
}

// AllowsHost reports whether host equals a pattern or is a subdomain of a
// *.domain pattern, ignoring case, a trailing dot, and IP address notation.
// host is compared in the ASCII form net/http dials.
func AllowsHost(patterns []string, host string) bool {
	host, ok := dialedHost(host)
	if !ok {
		return false
	}
	host = normalizeHost(host)
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "*.") {
			suffix := "." + normalizeHost(pattern[2:])
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		} else if host == normalizeHost(pattern) {
			return true
		}
	}
	return false
}

// dialedHost maps a non-ASCII host with the IDNA lookup rules net/http dials
// with; lowercasing the raw name instead equates names that resolve elsewhere.
func dialedHost(host string) (string, bool) {
	for i := 0; i < len(host); i++ {
		if host[i] >= utf8.RuneSelf {
			ascii, err := idna.Lookup.ToASCII(host)
			return ascii, err == nil
		}
	}
	return host, true
}

func normalizeHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.String()
	}
	return host
}
