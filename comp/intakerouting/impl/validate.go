// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	"errors"
	"fmt"
	"net"
	"strings"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
	pkgconfigutils "github.com/DataDog/datadog-agent/pkg/config/utils"
)

const (
	defaultHTTPSPort = 443
	maxHostnameLen   = 253
	maxLabelLen      = 63
)

// validateOverride checks that an endpoint returned by the backend is safe to send telemetry to:
// a hostname (no IP address, path, userinfo or port) under an official Datadog domain, of any
// site, that is not the bare site itself. It is always reached over HTTPS.
func validateOverride(host string, port int) error {
	if port < 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	if host == "" {
		return errors.New("empty host")
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return fmt.Errorf("host %q is an IP address", host)
	}
	name := normalizeHost(host)
	if len(name) > maxHostnameLen {
		return fmt.Errorf("host %q is too long", host)
	}
	for _, label := range strings.Split(name, ".") {
		if !isHostnameLabel(label) {
			return fmt.Errorf("host %q is not a valid hostname", host)
		}
	}
	site := pkgconfigutils.ExtractSiteFromURL("https://" + name)
	if site == "" {
		return fmt.Errorf("host %q is not under an official Datadog domain", host)
	}
	if name == site {
		return fmt.Errorf("host %q is a Datadog site, not an intake host", host)
	}
	return nil
}

// isHostnameLabel reports whether label is a valid DNS label made of ASCII letters, digits and
// hyphens. Anything else, such as '/', '@' or ':', rules out paths, userinfo and ports.
func isHostnameLabel(label string) bool {
	if label == "" || len(label) > maxLabelLen || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// normalizeHost lowercases host and removes one trailing dot (FQDN form).
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// normalizePort maps the default HTTPS port to 0, which means the same thing.
func normalizePort(port int) int {
	if port == defaultHTTPSPort {
		return 0
	}
	return port
}

// sameEndpoint reports whether a and b designate the same host and port.
func sameEndpoint(a, b intakerouting.Endpoint) bool {
	return normalizeHost(a.Host) == normalizeHost(b.Host) && normalizePort(a.Port) == normalizePort(b.Port)
}

// siteOf returns the Datadog site of host, or "" when host is not under an official Datadog
// domain.
func siteOf(host string) string {
	if host == "" {
		return ""
	}
	return pkgconfigutils.ExtractSiteFromURL("https://" + host)
}
