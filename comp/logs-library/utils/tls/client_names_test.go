// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tlsutil

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientNameMatcher(t *testing.T) {
	t.Run("no names yields an empty matcher", func(t *testing.T) {
		assert.True(t, newClientNameMatcher(nil).empty())
		assert.True(t, newClientNameMatcher([]string{}).empty())
	})

	t.Run("blank entries are dropped", func(t *testing.T) {
		assert.True(t, newClientNameMatcher([]string{"", "   "}).empty())

		m := newClientNameMatcher([]string{"relay.example.com", "  "})
		assert.False(t, m.empty())
		assert.Equal(t, []string{"relay.example.com"}, m.configured)
	})

	// Certificates may carry an empty subject alternative name. A blank entry
	// that survived into the matcher would authorize any such certificate.
	t.Run("blank entry does not authorize an empty subject alternative name", func(t *testing.T) {
		m := newClientNameMatcher([]string{"relay.example.com", ""})
		assert.Error(t, m.verify(&x509.Certificate{DNSNames: []string{""}}))
		assert.Error(t, m.verify(&x509.Certificate{EmailAddresses: []string{""}}))
	})

	t.Run("surrounding whitespace is ignored", func(t *testing.T) {
		m := newClientNameMatcher([]string{" relay.example.com "})
		cert := &x509.Certificate{DNSNames: []string{"relay.example.com"}}
		assert.NoError(t, m.verify(cert))
	})
}

func TestClientNameMatcherVerify(t *testing.T) {
	uri, err := url.Parse("spiffe://example.com/relay")
	require.NoError(t, err)

	cert := &x509.Certificate{
		Subject:        pkix.Name{CommonName: "relay-1"},
		DNSNames:       []string{"relay.example.com"},
		EmailAddresses: []string{"relay@example.com"},
		IPAddresses:    []net.IP{net.IPv4(10, 0, 0, 5)},
		URIs:           []*url.URL{uri},
	}

	allowedBy := []struct {
		name  string
		entry string
	}{
		{"dns SAN", "relay.example.com"},
		{"email SAN", "relay@example.com"},
		{"ip SAN", "10.0.0.5"},
		{"uri SAN", "spiffe://example.com/relay"},
		{"common name", "relay-1"},
	}
	for _, tc := range allowedBy {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, newClientNameMatcher([]string{"other", tc.entry}).verify(cert))
		})
	}

	// DNS names and the common name are compared without regard to case, so
	// operators need not match the CA's capitalization exactly.
	t.Run("dns and common name matching is case-insensitive", func(t *testing.T) {
		assert.NoError(t, newClientNameMatcher([]string{"RELAY.EXAMPLE.COM"}).verify(cert))
		assert.NoError(t, newClientNameMatcher([]string{"Relay-1"}).verify(cert))
	})

	// Only the scheme and authority of a URI are case-insensitive. Folding the
	// path would let one SPIFFE workload identity authorize a different one.
	t.Run("uri paths are case-sensitive", func(t *testing.T) {
		assert.Error(t, newClientNameMatcher([]string{"spiffe://example.com/Relay"}).verify(cert))
		assert.NoError(t, newClientNameMatcher([]string{"spiffe://example.com/relay"}).verify(cert))
	})

	// RFC 5321 leaves the local part case-sensitive while the domain is not.
	t.Run("only the mail domain is case-insensitive", func(t *testing.T) {
		mailOnly := &x509.Certificate{EmailAddresses: []string{"relay@example.com"}}
		assert.NoError(t, newClientNameMatcher([]string{"relay@EXAMPLE.COM"}).verify(mailOnly))
		assert.Error(t, newClientNameMatcher([]string{"Relay@example.com"}).verify(mailOnly))
	})

	// A certificate always presents its canonical form, so an operator writing
	// an uncompressed address should not be silently locked out.
	t.Run("ip entries are compared in canonical form", func(t *testing.T) {
		v6 := &x509.Certificate{IPAddresses: []net.IP{net.ParseIP("fe80::1")}}
		assert.NoError(t, newClientNameMatcher([]string{"FE80:0:0:0:0:0:0:1"}).verify(v6))
		assert.Error(t, newClientNameMatcher([]string{"fe80::2"}).verify(v6))
	})

	t.Run("unlisted identity is rejected", func(t *testing.T) {
		err := newClientNameMatcher([]string{"other.example.com"}).verify(cert)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "allowed_client_names")
		// The message must name both sides so the mismatch is diagnosable from
		// the Agent log alone.
		assert.Contains(t, err.Error(), "relay.example.com")
		assert.Contains(t, err.Error(), "other.example.com")
	})

	t.Run("substring of an allowed name is rejected", func(t *testing.T) {
		assert.Error(t, newClientNameMatcher([]string{"relay.example.com.attacker.test"}).verify(cert))
		assert.Error(t, newClientNameMatcher([]string{"example.com"}).verify(cert))
	})

	// Wildcards are not supported; an entry containing one matches literally
	// rather than silently authorizing a whole domain.
	t.Run("wildcard entry does not match a subdomain", func(t *testing.T) {
		assert.Error(t, newClientNameMatcher([]string{"*.example.com"}).verify(cert))
	})

	t.Run("certificate with no identities is rejected", func(t *testing.T) {
		assert.Error(t, newClientNameMatcher([]string{"relay-1"}).verify(&x509.Certificate{}))
	})
}

func TestCertificateNames(t *testing.T) {
	t.Run("blank common name is omitted", func(t *testing.T) {
		names := certificateNames(&x509.Certificate{
			Subject:  pkix.Name{CommonName: "  "},
			DNSNames: []string{"relay.example.com"},
		})
		assert.Equal(t, []string{"relay.example.com"}, names)
	})

	t.Run("no identities yields no names", func(t *testing.T) {
		assert.Empty(t, certificateNames(&x509.Certificate{}))
	})
}
