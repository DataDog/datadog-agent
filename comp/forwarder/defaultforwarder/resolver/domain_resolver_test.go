// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"net/http"
	"strings"
	"testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
		utils.NewAPIKeys("additional_endpoints", "example.com", "key1", "key2"),
		utils.NewAPIKeys("multi_region_failover.api_key", "example.com", "key2"),
	}

	resolver, err := NewSingleDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	assert.Equal(t, []string{"key1", "key2"}, resolver.GetAPIKeys())
}

func TestSingleDomainUpdateAPIKeys(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "example.com", "key1"),
		utils.NewAPIKeys("additional_endpoints", "example.com", "key1", "key2", "key3"),
	}

	resolver, err := NewSingleDomainResolver("example.com", apiKeys)
	require.NoError(t, err)

	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "example.com", "key4", "key2", "key3")})

	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)
}

func TestSingleDomainResolverAPIKeyNameRotation(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "key1", "key2"),
	}

	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)
	resolver.SetBaseDomain("https://7-0-0-agent.datadoghq.com")

	name, ok := resolver.GetAPIKeyName(0)
	require.True(t, ok)

	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "rotated-key1", "key2")})

	rotatedName, ok := resolver.GetAPIKeyName(0)
	require.True(t, ok)
	assert.Equal(t, name, rotatedName)
	idx, ok := resolver.GetAPIKeyIndex(name)
	require.True(t, ok)
	assert.Equal(t, uint(0), idx)
}

// TestSingleDomainResolverSetBaseDomainStableNames verifies that calling
// SetBaseDomain after construction does not rewrite already-stable names —
// once a key has a name, the name survives endpoint reassignment.
func TestSingleDomainResolverSetBaseDomainStableNames(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "key1", "key2"),
	}

	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)

	before := append([]string(nil), resolver.GetAPIKeyNames()...)

	resolver.SetBaseDomain("https://different.example.com")
	after := resolver.GetAPIKeyNames()

	assert.Equal(t, before, after,
		"SetBaseDomain must not rewrite existing API key names; identity must be stable")
}

// TestSingleDomainResolverPrimaryAndAdditionalShareKey verifies the dedup
// behavior when api_key shares its value with an additional_endpoints entry.
// The first occurrence wins, and the surviving name resolves back to index 0.
func TestSingleDomainResolverPrimaryAndAdditionalShareKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "https://app.datadoghq.com", "shared"),
		utils.NewAPIKeys("additional_endpoints", "https://app.datadoghq.com", "shared", "other"),
	}

	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)

	assertKeys(t, []string{"shared", "other"}, resolver)

	primaryName, ok := resolver.GetAPIKeyName(0)
	require.True(t, ok)
	require.NotEmpty(t, primaryName)
	idx, ok := resolver.GetAPIKeyIndex(primaryName)
	require.True(t, ok)
	assert.Equal(t, uint(0), idx)
}

// TestAuthorizeByNameUnknown verifies that an unknown name produces a clean
// error path and does not set the DD-Api-Key header.
func TestAuthorizeByNameUnknown(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "https://app.datadoghq.com", "key1"),
	}
	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)

	mockLog := logmock.New(t)
	headers := make(http.Header)
	resolver.AuthorizeByName("does-not-exist", headers, mockLog)
	assert.Empty(t, headers.Get("DD-Api-Key"),
		"unknown name must not authorize a request")
}

// TestAuthorizeUnknownIndex verifies the index-based path also fails cleanly
// when the index is out of range.
func TestAuthorizeUnknownIndex(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "https://app.datadoghq.com", "key1"),
	}
	resolver, err := NewSingleDomainResolver("https://app.datadoghq.com", apiKeys)
	require.NoError(t, err)

	mockLog := logmock.New(t)
	headers := make(http.Header)
	resolver.Authorize(99, headers, mockLog)
	assert.Empty(t, headers.Get("DD-Api-Key"))
}

func TestSingleDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "example.com", "key1"),
		utils.NewAPIKeys("additional_endpoints", "example.com", "key1", "key2", "key3"),
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
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assertKeys(t, []string{"key1", "key4", "key3"}, resolver)
}

func TestMultiDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "example.com", "key1"),
		utils.NewAPIKeys("additional_endpoints", "example.com", "key1", "key2", "key3"),
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
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assertKeys(t, []string{"key1", "key4", "key2", "key3"}, resolver)

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assertKeys(t, []string{"key1", "key4", "key3"}, resolver)
}

// TestUpdateAdditionalEndpointsPreservesEncName verifies that secret rotation
// does not change the stable enc_-prefixed name of an ENC-backed additional
// endpoint key. Before the fix, updateAdditionalEndpoints passed nil for the
// raw-config view so MakeNamedEndpoints only saw resolved key material and
// regenerated the name as idx_..., breaking persisted enc_ references.
func TestUpdateAdditionalEndpointsPreservesEncName(t *testing.T) {
	const (
		endpoint   = "https://app.datadoghq.com"
		setting    = "additional_endpoints"
		initialKey = "initial_key_material"
		rotatedKey = "rotated_key_material"
	)

	// YAML load populates the file layer with the raw ENC[...] handle,
	// mirroring how the agent reads datadog.yaml before secret resolution.
	mockCfg := configmock.NewFromYAML(t, `
additional_endpoints:
  "https://app.datadoghq.com":
  - ENC[my_api_handle]
`)
	// Secret resolution writes the resolved material into the secrets layer.
	mockCfg.Set(setting, map[string]interface{}{
		endpoint: []interface{}{initialKey},
	}, model.SourceSecret)

	// Build the initial resolver the same way GetEndpointsFromConfig does:
	// resolved view for key material, raw view to preserve the enc_ name.
	apiKeysByDomain := utils.MakeNamedEndpoints(
		mockCfg.GetStringMapStringSlice(setting),
		utils.RawStringMapStringSlice(mockCfg, setting),
		setting,
	)
	resolver, err := NewSingleDomainResolver(endpoint, apiKeysByDomain[endpoint])
	require.NoError(t, err)

	initialName, ok := resolver.GetAPIKeyName(0)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(initialName, "enc_"), "expected enc_ prefix before rotation, got %q", initialName)

	// Rotate: file layer (ENC[...] handle) is unchanged; only the secrets
	// layer is updated with the new key material.
	mockCfg.Set(setting, map[string]interface{}{
		endpoint: []interface{}{rotatedKey},
	}, model.SourceSecret)

	log := logmock.New(t)
	updateAdditionalEndpoints(resolver, setting, mockCfg, log)

	rotatedName, ok := resolver.GetAPIKeyName(0)
	require.True(t, ok)
	assert.Equal(t, initialName, rotatedName, "ENC-backed key name must not change on secret rotation")
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
