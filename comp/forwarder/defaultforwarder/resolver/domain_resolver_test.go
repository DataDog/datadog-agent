// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"net/url"
	"testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertKeys(t *testing.T, expect []string, resolver DomainResolver) {
	expectHdr := make([]authHeader, len(expect))
	for i, key := range expect {
		expectHdr[i] = authHeader{"DD-Api-Key", key}
	}
	assert.Equal(t, expect, resolver.GetAPIKeys())
	assert.Equal(t, expectHdr, resolver.GetAuthorizers())
}

func TestSingleDomainResolverDedupedKey(t *testing.T) {
	// Note key2 exists twice in the list.
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "key1", "key2"),
		utils.NewAPIKeys("multi_region_failover.api_key", "key2"),
	}

	resolver, err := NewSingleDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	assert.Equal(t, resolver.dedupedAPIKeys,
		[]string{"key1", "key2"})
}

// TestIsUsableKeepsPendingDelegatedAuthDomain is a regression test: a domain whose only entry is
// a pending DELA(...) directive has zero real API keys, but must still be usable. Otherwise
// default_forwarder.go drops it from the forwarder entirely at startup (see IsUsable's callers),
// and once delegatedauth resolves the directive and writes the real key back into config, there is
// no registered resolver left to receive that update - the key is silently lost until restart.
func TestIsUsableKeepsPendingDelegatedAuthDomain(t *testing.T) {
	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL: "https://pending-org.datadoghq.com",
		APIKeySet: []utils.APIKeys{
			{ConfigSettingPath: "additional_endpoints", Keys: []string{}, HasPendingDelegatedAuth: true},
		},
	})
	require.NoError(t, err)

	assert.True(t, resolver.IsUsable())
}

// TestIsUsableDropsDomainWithNoKeysAndNoPendingDirective is a regression guard for the above fix:
// a domain with genuinely zero API keys and no pending delegated-auth directive must still be
// considered unusable, unchanged from prior behavior.
func TestIsUsableDropsDomainWithNoKeysAndNoPendingDirective(t *testing.T) {
	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL: "https://empty-org.datadoghq.com",
		APIKeySet: []utils.APIKeys{
			{ConfigSettingPath: "additional_endpoints", Keys: []string{}},
		},
	})
	require.NoError(t, err)

	assert.False(t, resolver.IsUsable())
}

// TestUpdateAPIKeysRefreshesPendingDelegatedAuthFlag is a regression test: once delegatedauth
// resolves a directive and writes the real key back into config, UpdateAPIKeys (invoked via the
// config OnUpdate callback) must clear hasPendingDelegatedAuth - otherwise the resolver would look
// permanently pending even after it has a real usable key.
func TestUpdateAPIKeysRefreshesPendingDelegatedAuthFlag(t *testing.T) {
	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL: "https://resolving-org.datadoghq.com",
		APIKeySet: []utils.APIKeys{
			{ConfigSettingPath: "additional_endpoints", Keys: []string{}, HasPendingDelegatedAuth: true},
		},
	})
	require.NoError(t, err)
	require.True(t, resolver.hasPendingDelegatedAuth)

	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{
		{ConfigSettingPath: "additional_endpoints", Keys: []string{"resolved-key"}},
	})

	assert.False(t, resolver.hasPendingDelegatedAuth)
	assert.True(t, resolver.IsUsable())
}

func TestSingleDomainUpdateAPIKeys(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}

	resolver, err := NewSingleDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "key4", "key2", "key3")})

	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)
}

func TestSingleDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}
	resolver, err := NewSingleDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	// The duplicate key between the main endpoint and additional_endpoints is removed
	assertKeys(t, []string{"key1", "key2", "key3"}, resolver)

	log := logmock.New(t)
	mockConfig := configmock.New(t)
	endpoints := map[string][]string{
		"example.com": {"key4", "key2", "key3"},
	}
	mockConfig.SetInTest("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetInTest("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assertKeys(t, []string{"key1", "key4", "key3"}, resolver)
}

func TestSingleDomainResolverUpdateAdditionalEndpointsAfterBaseDomainRewrite(t *testing.T) {
	// Regression test: NewDefaultForwarder rewrites a resolver's base domain via SetBaseDomain
	// (AddAgentVersionToDomain, e.g. "https://app.datadoghq.com" -> a version-prefixed host) for
	// well-known Datadog domains, including additional_endpoints ones - but the additional_endpoints
	// config map stays keyed by the original, unrewritten URL. updateAdditionalEndpoints must look
	// up by GetConfigName() (unchanged by the rewrite), not GetBaseDomain(), or it silently discards
	// every additional_endpoints update - including a key resolved by delegated auth - for any such
	// domain.
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "key1"),
	}
	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)

	// Simulate NewDefaultForwarder's AddAgentVersionToDomain rewrite: the base domain used for
	// network requests diverges from the domain as configured.
	resolver.SetBaseDomain("https://7-65-0.agent.datadoghq.com")
	require.Equal(t, "https://app.datadoghq.com", resolver.GetConfigName())
	require.Equal(t, "https://7-65-0.agent.datadoghq.com", resolver.GetBaseDomain())

	log := logmock.New(t)
	mockConfig := configmock.New(t)
	// Keyed by the original configured URL, exactly as the user wrote it / as delegated auth
	// would write a resolved key back into it - never by the rewritten base domain.
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://app.datadoghq.com": {"key2"},
	})
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assertKeys(t, []string{"key2"}, resolver)
}

func TestMultiDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}
	resolver, err := NewMultiDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	// The duplicate key between the main endpoint and additional_endpoints is removed
	assertKeys(t, []string{"key1", "key2", "key3"}, resolver)

	log := logmock.New(t)
	mockConfig := configmock.New(t)
	endpoints := map[string][]string{
		"example.com": {"key4", "key2", "key3"},
	}
	mockConfig.SetInTest("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetInTest("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assertKeys(t, []string{"key1", "key4", "key3"}, resolver)
}

func TestIsMetricToVector(t *testing.T) {
	apiKeys := []utils.APIKeys{utils.NewAPIKeys("api_key", "key1")}

	plain, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)
	assert.False(t, plain.IsMetricToVector(), "plain resolver must report no vector override")

	multi, err := NewMultiDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)
	assert.False(t, multi.IsMetricToVector(), "multi-domain resolver without vector divert must report false")

	vec, err := NewDomainResolverWithMetricToVector(
		"https://app.datadoghq.com",
		apiKeys,
		"http://vector.example.test:8080",
	)
	require.NoError(t, err)
	assert.True(t, vec.IsMetricToVector(), "vector-diverted resolver must report true")
	assert.Equal(t, "https://app.datadoghq.com", vec.GetConfigName())
}

func TestMetricToVectorResolvesSeriesEndpoints(t *testing.T) {
	const mainEndpoint = "https://app.datadoghq.com"
	const vectorEndpoint = "http://vector.example.test:8080"
	apiKeys := []utils.APIKeys{utils.NewAPIKeys("api_key", "key1")}

	vec, err := NewDomainResolverWithMetricToVector(mainEndpoint, apiKeys, vectorEndpoint)
	require.NoError(t, err)

	// All series and sketch endpoints, including v3, must be diverted to vector.
	for _, endpoint := range []transaction.Endpoint{
		endpoints.V1SeriesEndpoint,
		endpoints.SeriesEndpoint,
		endpoints.V3SeriesEndpoint,
		endpoints.SketchSeriesEndpoint,
	} {
		assert.Equal(t, vectorEndpoint, vec.Resolve(endpoint), "%s must be diverted to vector", endpoint.Name)
	}

	// Unrelated endpoints stay on the main Datadog domain.
	assert.Equal(t, mainEndpoint, vec.Resolve(endpoints.EventsEndpoint))
}

func TestLegacyEndpointConversionPreservesPendingDelegatedAuth(t *testing.T) {
	endpoint, err := url.Parse("https://pending.example")
	require.NoError(t, err)

	resolvers, err := NewSingleDomainResolvers(apicfg.KeysPerDomains([]apicfg.Endpoint{{
		Endpoint:                endpoint,
		ConfigSettingPath:       "process_config.additional_endpoints",
		HasPendingDelegatedAuth: true,
	}}))
	require.NoError(t, err)
	assert.True(t, resolvers["https://pending.example"].IsUsable())
}

func TestIsUsableWithNoKeysAndNoPendingDelegatedAuth(t *testing.T) {
	// Baseline: a domain with no real keys and no pending delegated auth directive is not
	// usable, same as before HasPendingDelegatedAuth existed.
	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:   "https://example.com",
		APIKeySet: []utils.APIKeys{{ConfigSettingPath: "additional_endpoints", Keys: []string{}}},
	})
	require.NoError(t, err)

	assert.False(t, resolver.IsUsable())
}

func TestIsUsableWithPendingDelegatedAuth(t *testing.T) {
	// A domain with no real keys yet, but flagged as waiting on a delegatedauth-managed key
	// (a DELA(...) directive in additional_endpoints), must still be usable so the forwarder
	// builds a live domainForwarder for it instead of dropping it until an agent restart.
	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:                 "https://example.com",
		APIKeySet:               []utils.APIKeys{{ConfigSettingPath: "additional_endpoints", Keys: []string{}}},
		HasPendingDelegatedAuth: true,
	})
	require.NoError(t, err)

	assert.True(t, resolver.IsUsable())

	// Once delegated auth delivers a real key, the domain remains usable through the normal
	// UpdateAPIKeys path (unrelated to hasPendingDelegatedAuth, which is only a startup fallback).
	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "real-key")})
	assert.True(t, resolver.IsUsable())
	assertKeys(t, []string{"real-key"}, resolver)
}

func TestScrubKeys(t *testing.T) {
	keys := []string{
		"abcdefghijklmnopqrstuvwxyzkey001",
		"abcdefghijklmnopqrstuvwxyzkey002",
		"shortkey",
	}
	keys = scrubKeys(keys)

	assert.Equal(t, []string{"****************************y001", "****************************y002", "******ey"}, keys)
}
