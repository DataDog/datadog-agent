// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
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

func TestPendingDelegatedAuthGetsAtLeastOneAuthorizer(t *testing.T) {
	// A domain whose only key source is a pending DELA(...) directive must still produce at
	// least one authorizer/transaction - otherwise it never receives a 403 from the backend and
	// the retry-not-drop path in transaction.go is never reached. utils.MakeEndpoints keeps the
	// directive text itself as a placeholder key for exactly this reason.
	keysPerDomain := utils.MakeEndpoints(map[string][]string{
		"https://example.com": {"DELA(some-org-uuid, aws)"},
	}, "additional_endpoints")

	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:                 "https://example.com",
		APIKeySet:               keysPerDomain["https://example.com"],
		HasPendingDelegatedAuth: true,
	})
	require.NoError(t, err)

	assertKeys(t, []string{"DELA(some-org-uuid, aws)"}, resolver)

	// Once delegated auth resolves the real key, UpdateAPIKeys replaces the whole config-path
	// entry, so the placeholder cannot linger alongside the real key.
	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "real-key")})
	assertKeys(t, []string{"real-key"}, resolver)
}

// TestHasPendingDelegatedAuthPerKey proves that a domain mixing a static key with a still-pending
// DELA(...) placeholder tracks "pending" per key/authorizer index, not once for the whole domain.
// A bad static key sitting alongside a DELA-managed key on the same domain must not be treated as
// transient just because the domain also has a pending delegated-auth key - see
// transaction.PendingDelegatedAuthChecker.
func TestHasPendingDelegatedAuthPerKey(t *testing.T) {
	keysPerDomain := utils.MakeEndpoints(map[string][]string{
		"https://example.com": {"some-static-key", "DELA(some-org-uuid, aws)"},
	}, "additional_endpoints")

	resolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:                 "https://example.com",
		APIKeySet:               keysPerDomain["https://example.com"],
		HasPendingDelegatedAuth: true,
	})
	require.NoError(t, err)

	assertKeys(t, []string{"some-static-key", "DELA(some-org-uuid, aws)"}, resolver)

	assert.False(t, resolver.HasPendingDelegatedAuth(0), "the static key's index must not be treated as delegated-auth pending")
	assert.True(t, resolver.HasPendingDelegatedAuth(1), "the DELA(...) placeholder's index must be treated as delegated-auth pending")
	assert.False(t, resolver.HasPendingDelegatedAuth(2), "an out-of-range index must not be treated as delegated-auth pending")

	// Once delegated auth resolves the real key, UpdateAPIKeys replaces the whole config-path's
	// keys (both the static key and the placeholder live under the same "additional_endpoints"
	// config path), so the previously-pending index must stop being treated as pending too, and
	// the alignment between dedupedAPIKeys and the pending check must not drift.
	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "some-static-key", "real-key")})
	assertKeys(t, []string{"some-static-key", "real-key"}, resolver)
	assert.False(t, resolver.HasPendingDelegatedAuth(0), "the static key's index must still not be treated as delegated-auth pending after an update")
	assert.False(t, resolver.HasPendingDelegatedAuth(1), "once resolved, the key at that index is no longer a placeholder")
}

// TestTransactionProcess403PerKeyDelegatedAuthPending exercises the full 403-handling path in
// transaction.go against a domainResolver that mixes a bad static key with a still-pending
// DELA(...) placeholder: a 403 against the static key's authorizer index must drop the
// transaction normally, while a 403 against the DELA(...) placeholder's index must be retried and
// nudge delegated auth to refresh sooner.
func TestTransactionProcess403PerKeyDelegatedAuthPending(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	keysPerDomain := utils.MakeEndpoints(map[string][]string{
		"https://example.com": {"some-static-key", "DELA(some-org-uuid, aws)"},
	}, "additional_endpoints")

	domResolver, err := NewSingleDomainResolver2(utils.EndpointDescriptor{
		BaseURL:                 "https://example.com",
		APIKeySet:               keysPerDomain["https://example.com"],
		HasPendingDelegatedAuth: true,
	})
	require.NoError(t, err)

	newTransaction := func(apiKeyIdx uint) *transaction.HTTPTransaction {
		tr := transaction.NewHTTPTransaction()
		tr.Domain = ts.URL
		tr.Endpoint.Route = "/endpoint/test"
		tr.Endpoint.Name = "test"
		tr.Payload = transaction.NewBytesPayload([]byte("test payload"), 1)
		tr.Resolver = domResolver
		tr.APIKeyIndex = apiKeyIdx
		return tr
	}

	secrets := secretsmock.New(t)

	// Index 0 is the static key - a 403 must drop the transaction, not reschedule it, and must not
	// nudge delegated auth.
	var refreshCalledForStatic bool
	staticDelegatedAuth := &delegatedauthmock.Mock{
		RefreshFunc: func() bool {
			refreshCalledForStatic = true
			return true
		},
	}
	err = newTransaction(0).Process(context.Background(), configmock.New(t), logmock.New(t), secrets, staticDelegatedAuth, &http.Client{}, nil)
	assert.NoError(t, err, "a 403 on a static key must drop the transaction, not reschedule it")
	assert.False(t, refreshCalledForStatic, "a 403 on a static key must not nudge delegated auth")

	// Index 1 is the DELA(...) placeholder - a 403 must be retried and nudge delegated auth.
	var refreshCalledForDela bool
	delaDelegatedAuth := &delegatedauthmock.Mock{
		RefreshFunc: func() bool {
			refreshCalledForDela = true
			return true
		},
	}
	err = newTransaction(1).Process(context.Background(), configmock.New(t), logmock.New(t), secrets, delaDelegatedAuth, &http.Client{}, nil)
	assert.Error(t, err, "a 403 on a still-pending delegated-auth key must be retried")
	assert.True(t, refreshCalledForDela, "a 403 on the pending key should nudge delegated auth to refresh sooner")
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
