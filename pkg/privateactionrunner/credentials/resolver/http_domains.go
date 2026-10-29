// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// validateHTTPDomains rejects URLs, ports, and ambiguous wildcard patterns.
func validateHTTPDomains(domains []string) error {
	for _, pattern := range domains {
		host := strings.TrimSuffix(strings.ToLower(pattern), ".")
		wildcard := strings.HasPrefix(host, "*.")
		host = strings.TrimPrefix(host, "*.")
		if ip, err := netip.ParseAddr(host); err == nil {
			if wildcard || ip.Zone() != "" {
				return errors.New("invalid HTTP domain pattern")
			}
			continue
		}
		if len(host) == 0 || len(host) > 253 {
			return errors.New("invalid HTTP domain pattern")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return errors.New("invalid HTTP domain pattern")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return errors.New("invalid HTTP domain pattern")
				}
			}
		}
	}
	return nil
}

// allowsHTTPURL compares parsed hostnames, never URL prefixes or DNS results.
func allowsHTTPURL(domains []string, u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.Hostname() == "" {
		return false
	}
	host := normalizeHTTPHost(u.Hostname())
	for _, pattern := range domains {
		if strings.HasPrefix(pattern, "*.") {
			suffix := "." + normalizeHTTPHost(pattern[2:])
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		} else if host == normalizeHTTPHost(pattern) {
			return true
		}
	}
	return false
}

func normalizeHTTPHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.String()
	}
	return host
}
