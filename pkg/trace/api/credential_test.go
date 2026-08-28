// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// Normal API keys in evp_proxy endpoints must be unaffected by the provider path.
// The endpoint has no CredentialProvider, so authorizeEndpoint stamps the static key.
func TestEVPProxyNormalAPIKeyUnaffected(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.EVPProxy.DDURL = "https://example.com"
	conf.EVPProxy.APIKey = "plain-key"

	endpoints := evpProxyEndpointsFromConfig(conf)
	require.Len(t, endpoints, 1)
	assert.Nil(t, endpoints[0].CredentialProvider, "a normal key must not get a provider")

	h := http.Header{}
	require.True(t, authorizeEndpoint(endpoints[0], h), "a normal key must authorize")
	assert.Equal(t, "plain-key", h.Get("DD-API-KEY"))
}

// ENC[...] keys resolved by the secrets backend are normal static keys from
// the proxy's perspective. The provider path must not interfere.
func TestEVPProxyResolvedEncKeyUnaffected(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.EVPProxy.DDURL = "https://example.com"
	conf.EVPProxy.APIKey = "resolved-enc-key"

	endpoints := evpProxyEndpointsFromConfig(conf)
	require.Len(t, endpoints, 1)
	assert.Nil(t, endpoints[0].CredentialProvider, "a resolved ENC[] key must not get a provider")

	h := http.Header{}
	require.True(t, authorizeEndpoint(endpoints[0], h), "a resolved ENC[] key must authorize")
	assert.Equal(t, "resolved-enc-key", h.Get("DD-API-KEY"))
}

// A DELA directive endpoint with a resolved provider authorizes via the provider.
// A normal key alongside it is unaffected.
func TestEVPProxyMixedKeysAndDirective(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.EVPProxy.DDURL = "https://example.com"
	conf.EVPProxy.APIKey = "plain-key"
	conf.EVPProxy.AdditionalEndpoints = map[string][]string{
		"https://additional.com": {"DELA(org-uuid, aws)"},
	}
	conf.CredentialProviderFn = func(_, _, _ string) config.CredentialProvider {
		p := &delegatedauthmock.StubProvider{Key: "delegated-key"}
		p.SetReady(true)
		return p
	}

	endpoints := evpProxyEndpointsFromConfig(conf)
	require.Len(t, endpoints, 2)

	// Main endpoint: normal key, no provider
	assert.Nil(t, endpoints[0].CredentialProvider)
	h0 := http.Header{}
	require.True(t, authorizeEndpoint(endpoints[0], h0))
	assert.Equal(t, "plain-key", h0.Get("DD-API-KEY"))

	// Additional endpoint: directive, has provider
	require.NotNil(t, endpoints[1].CredentialProvider, "a DELA directive must get a provider")
	assert.Empty(t, endpoints[1].APIKey, "the directive text must not leak as a key")
	h1 := http.Header{}
	require.True(t, authorizeEndpoint(endpoints[1], h1))
	assert.Equal(t, "delegated-key", h1.Get("DD-API-KEY"))
}

// A DELA directive with no provider (unsupported cloud, or no lookup wired)
// must refuse rather than send unauthenticated.
func TestEVPProxyDirectiveWithNoProviderRefuses(t *testing.T) {
	conf := newTestReceiverConfig()
	conf.EVPProxy.DDURL = "https://example.com"
	conf.EVPProxy.APIKey = "plain-key"
	conf.EVPProxy.AdditionalEndpoints = map[string][]string{
		"https://additional.com": {"DELA(org-uuid, aws)"},
	}
	// No CredentialProviderFn set

	endpoints := evpProxyEndpointsFromConfig(conf)
	require.Len(t, endpoints, 2)

	// The directive endpoint has no provider and no key
	assert.Empty(t, endpoints[1].APIKey)
	assert.Nil(t, endpoints[1].CredentialProvider)

	h := http.Header{}
	assert.False(t, authorizeEndpoint(endpoints[1], h), "a directive with no provider must not authorize")
	assert.Empty(t, h, "nothing may be stamped without a credential")
}
