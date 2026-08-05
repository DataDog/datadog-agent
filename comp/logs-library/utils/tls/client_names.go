// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"crypto/x509"
	"fmt"
	"strings"
)

// clientNameMatcher authorizes a client by the identity in its certificate,
// narrowing access from "anyone the CA vouches for" to a named set of clients.
// A shared or organization-wide CA otherwise admits every certificate it has
// ever issued, which is rarely the intended trust boundary for a log listener.
type clientNameMatcher struct {
	// allowed holds the lowercased entries used for lookups.
	allowed map[string]struct{}
	// configured keeps the original spellings so rejection messages show what
	// the operator actually wrote.
	configured []string
}

// newClientNameMatcher builds a matcher from the configured names. Blank
// entries are ignored so that a trailing comma in a comma-separated list does
// not create an unmatchable entry.
func newClientNameMatcher(names []string) *clientNameMatcher {
	m := &clientNameMatcher{allowed: make(map[string]struct{}, len(names))}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		m.allowed[strings.ToLower(name)] = struct{}{}
		m.configured = append(m.configured, name)
	}
	return m
}

// empty reports whether the matcher would authorize nothing, which means the
// caller should not install it at all.
func (m *clientNameMatcher) empty() bool {
	return len(m.allowed) == 0
}

// verify accepts the certificate when any identity it presents is allowed.
func (m *clientNameMatcher) verify(cert *x509.Certificate) error {
	presented := certificateNames(cert)
	for _, name := range presented {
		if _, ok := m.allowed[strings.ToLower(name)]; ok {
			return nil
		}
	}
	return fmt.Errorf("client certificate identities [%s] do not match any entry in allowed_client_names [%s]",
		strings.Join(presented, ", "), strings.Join(m.configured, ", "))
}

// certificateNames returns the identities a client certificate presents: every
// subject alternative name, plus the subject common name.
//
// Hostname verification (RFC 6125) ignores the common name whenever any
// subject alternative name is present. That rule is deliberately not applied
// here: this is client authorization rather than server identity checking, and
// client certificates in the field routinely carry their identity only in the
// common name. Any name the trusted CA chose to put in the certificate is
// therefore eligible to match.
func certificateNames(cert *x509.Certificate) []string {
	names := make([]string, 0, len(cert.DNSNames)+len(cert.EmailAddresses)+len(cert.IPAddresses)+len(cert.URIs)+1)
	names = append(names, cert.DNSNames...)
	names = append(names, cert.EmailAddresses...)
	for _, ip := range cert.IPAddresses {
		names = append(names, ip.String())
	}
	for _, uri := range cert.URIs {
		names = append(names, uri.String())
	}
	if cn := strings.TrimSpace(cert.Subject.CommonName); cn != "" {
		names = append(names, cn)
	}
	return names
}
