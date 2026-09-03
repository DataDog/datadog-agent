// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"crypto/x509"
	"fmt"
	"net"
	"strings"
)

// clientNameMatcher authorizes a client by the identity in its certificate,
// narrowing access from "anyone the CA vouches for" to a named set of clients.
// A shared or organization-wide CA otherwise admits every certificate it has
// ever issued, which is rarely the intended trust boundary for a log listener.
//
// Each configured entry is indexed under every comparison rule, because the
// identity type an operator had in mind is not known until a certificate
// presents one. Comparison then follows the rule for the type actually
// presented, so no identity is matched more loosely than its own syntax allows.
type clientNameMatcher struct {
	// folded holds entries canonicalized for the identity types compared
	// without regard to case: DNS names, IP addresses and the common name.
	folded map[string]struct{}
	// mailbox holds entries with only their domain folded, for email addresses.
	mailbox map[string]struct{}
	// exact holds entries verbatim, for URIs.
	exact map[string]struct{}
	// configured keeps the original spellings so rejection messages show what
	// the operator actually wrote.
	configured []string
}

// newClientNameMatcher builds a matcher from the configured names.
//
// Blank entries are dropped. Configuration validation already rejects them, so
// this guards the case where a matcher is built directly: an empty entry would
// otherwise authorize a certificate carrying an empty subject alternative name.
func newClientNameMatcher(names []string) *clientNameMatcher {
	m := &clientNameMatcher{
		folded:  make(map[string]struct{}, len(names)),
		mailbox: make(map[string]struct{}, len(names)),
		exact:   make(map[string]struct{}, len(names)),
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		m.folded[foldIdentity(name)] = struct{}{}
		m.mailbox[foldMailboxDomain(name)] = struct{}{}
		m.exact[name] = struct{}{}
		m.configured = append(m.configured, name)
	}
	return m
}

// empty reports whether the matcher would authorize nothing, which means the
// caller should not install it at all.
func (m *clientNameMatcher) empty() bool {
	return len(m.configured) == 0
}

// verify accepts the certificate when any identity it presents is allowed.
func (m *clientNameMatcher) verify(cert *x509.Certificate) error {
	for _, name := range cert.DNSNames {
		if contains(m.folded, foldIdentity(name)) {
			return nil
		}
	}
	for _, email := range cert.EmailAddresses {
		if contains(m.mailbox, foldMailboxDomain(email)) {
			return nil
		}
	}
	for _, ip := range cert.IPAddresses {
		if contains(m.folded, foldIdentity(ip.String())) {
			return nil
		}
	}
	for _, uri := range cert.URIs {
		if contains(m.exact, uri.String()) {
			return nil
		}
	}
	if cn := strings.TrimSpace(cert.Subject.CommonName); cn != "" {
		if contains(m.folded, foldIdentity(cn)) {
			return nil
		}
	}

	presented := certificateNames(cert)
	return fmt.Errorf("client certificate identities [%s] do not match any entry in allowed_client_names [%s]",
		strings.Join(presented, ", "), strings.Join(m.configured, ", "))
}

func contains(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// foldIdentity canonicalizes an identity whose comparison ignores case: DNS
// names (RFC 4343), IP addresses, and the common name, for which X.500
// prescribes caseIgnoreMatch. IP literals are also reduced to their canonical
// textual form, so an operator who writes an uncompressed IPv6 address still
// matches the address a certificate presents.
func foldIdentity(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return strings.ToLower(s)
}

// foldMailboxDomain folds only the domain of an email address. RFC 5321 leaves
// the local part case-sensitive, and Go draws the same line when matching email
// name constraints.
func foldMailboxDomain(s string) string {
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return s
	}
	return s[:at+1] + strings.ToLower(s[at+1:])
}

// certificateNames returns the identities a client certificate presents: every
// subject alternative name, plus the subject common name. It backs the
// rejection message; authorization itself compares each identity under the rule
// for its own type.
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
