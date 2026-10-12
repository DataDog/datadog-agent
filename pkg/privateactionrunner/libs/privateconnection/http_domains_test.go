// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateconnection

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDomainAllowlist(t *testing.T) {
	for _, test := range []struct {
		pattern, target string
		allowed         bool
	}{
		{"api.example.com", "https://api.example.com/path?secret=value", true},
		{"API.EXAMPLE.COM.", "http://api.example.com.:8080/path", true},
		{"api.example.com", "https://api.example.com.evil.test", false},
		{"api.example.com", "https://api.example.com@evil.test", false},
		{"api.example.com", "https://evil.test/api.example.com", false},
		{"api.example.com", "https://sub.api.example.com", false},
		{"*.example.com", "https://api.example.com", true},
		{"*.EXAMPLE.COM", "https://nested.api.example.com", true},
		{"*.example.com", "https://example.com", false},
		{"*.example.com", "https://notexample.com", false},
		{"*.example.com", "https://example.com.evil.test", false},
		{"api.example.com", "//api.example.com/path", false},
		{"api.example.com", "ftp://api.example.com", false},
		{"api.example.com", "https:api.example.com", false},
		{"127.0.0.1", "http://127.0.0.1:8080", true},
		{"::1", "http://[0:0:0:0:0:0:0:1]:8080", true},
		{"xn--bcher-kva.example", "https://xn--bcher-kva.example", true},
	} {
		t.Run(test.pattern+"/"+test.target, func(t *testing.T) {
			domains := HTTPDomainAllowlist{test.pattern}
			require.NoError(t, domains.Validate())
			u, err := url.Parse(test.target)
			require.NoError(t, err)
			assert.Equal(t, test.allowed, domains.AllowsURL(u))
		})
	}
}

func TestInvalidHTTPDomainPatterns(t *testing.T) {
	for _, pattern := range []string{"", "*", "https://example.com", "example.com:443", "example.com/path", "user@example.com", "foo.*.com", "*example.com", "example..com", "example.com..", "-bad.example", "bad-.example", " example.com", "example.com ", "[::1]", "*.127.0.0.1", "fe80::1%eth0", "bücher.example"} {
		t.Run(pattern, func(t *testing.T) {
			require.Error(t, HTTPDomainAllowlist{pattern}.Validate())
		})
	}
}

func TestHTTPDomainRestrictionsIntersect(t *testing.T) {
	u, err := url.Parse("https://api.example.com")
	require.NoError(t, err)
	var credentials *PrivateCredentials
	require.NoError(t, credentials.ValidateHTTPURL(u))
	credentials = &PrivateCredentials{}
	require.NoError(t, credentials.ValidateHTTPURL(u))
	credentials.HTTPDomainRestrictions = []HTTPDomainAllowlist{{"*.example.com"}, {"api.example.com"}}
	require.NoError(t, credentials.ValidateHTTPURL(u))
	credentials.HTTPDomainRestrictions = append(credentials.HTTPDomainRestrictions, HTTPDomainAllowlist{"other.example.com"})
	require.ErrorIs(t, credentials.ValidateHTTPURL(u), ErrHTTPDomainNotAllowed)
	credentials.HTTPDomainRestrictions = []HTTPDomainAllowlist{{}}
	require.ErrorIs(t, credentials.ValidateHTTPURL(u), ErrHTTPDomainNotAllowed)
	require.ErrorIs(t, credentials.ValidateHTTPURL(nil), ErrHTTPDomainNotAllowed)
}
