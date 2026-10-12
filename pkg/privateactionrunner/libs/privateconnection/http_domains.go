// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateconnection

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// HTTPDomainAllowlist permits exact hostnames/IPs or subdomains of a *.domain
// pattern. Ports and schemes are not part of a domain restriction.
type HTTPDomainAllowlist []string

// ErrHTTPDomainNotAllowed contains no URL or secret values.
var ErrHTTPDomainNotAllowed = errors.New("request domain is not allowed by runner credential policy")

// Validate rejects URLs, ports, and ambiguous wildcard patterns.
func (domains HTTPDomainAllowlist) Validate() error {
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

// AllowsURL compares parsed hostnames, never URL prefixes or DNS results.
func (domains HTTPDomainAllowlist) AllowsURL(u *url.URL) bool {
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

// ValidateHTTPURL requires all credential restrictions to permit the URL.
// Errors deliberately omit URLs, which may contain credential values.
func (p *PrivateCredentials) ValidateHTTPURL(u *url.URL) error {
	if p == nil {
		return nil
	}
	for _, domains := range p.HTTPDomainRestrictions {
		if !domains.AllowsURL(u) {
			return ErrHTTPDomainNotAllowed
		}
	}
	return nil
}

// TestURL derives the connection-test destination consistently for credential
// authorization and HTTP execution. A test path may itself be an absolute URL.
func (h HttpDetails) TestURL() (*url.URL, error) {
	if h.Testing == nil || h.BaseURL == "" {
		return nil, errors.New("connection does not have a base URL and testing configuration")
	}
	base, err := url.Parse(h.BaseURL)
	if err != nil {
		return nil, errors.New("invalid connection base URL")
	}
	u, err := base.Parse(h.Testing.Path)
	if err != nil {
		return nil, errors.New("invalid connection testing path")
	}
	return u, nil
}
