// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"testing"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/assert"
)

func TestSingleDomainResolverDedupedKey(t *testing.T) {
	// Note key2 exists twice in the list.
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "key1", "key2"),
		utils.NewAPIKeys("multi_region_failover.api_key", "key2"),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	assert.Equal(t, resolver.dedupedAPIKeys,
		[]string{"key1", "key2"})
}

func TestSingleDomainUpdateAPIKeys(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	resolver.UpdateAPIKeys("additional_endpoints", []utils.APIKeys{utils.NewAPIKeys("additional_endpoints", "key4", "key2", "key3")})

	assert.Equal(t, []string{"key1", "key4", "key2", "key3"}, resolver.GetAPIKeys())
}

func TestSingleDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}
	resolver := NewSingleDomainResolver("example.com", apiKeys)

	// The duplicate key between the main endpoint and additional_endpoints is removed
	assert.Equal(t, []string{"key1", "key2", "key3"}, resolver.GetAPIKeys())

	log := logmock.New(t)
	mockConfig := configmock.New(t)
	endpoints := map[string][]string{
		"example.com": {"key4", "key2", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assert.Equal(t, []string{"key1", "key4", "key2", "key3"}, resolver.GetAPIKeys())

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assert.Equal(t, []string{"key1", "key4", "key3"}, resolver.GetAPIKeys())
}

func TestMultiDomainResolverUpdateAdditionalEndpointsNewKey(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("api_key", "key1"),
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3"),
	}
	resolver := NewMultiDomainResolver("example.com", apiKeys)

	// The duplicate key between the main endpoint and additional_endpoints is removed
	assert.Equal(t, []string{"key1", "key2", "key3"}, resolver.GetAPIKeys())

	log := logmock.New(t)
	mockConfig := configmock.New(t)
	endpoints := map[string][]string{
		"example.com": {"key4", "key2", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	// The new key4 key is in the list and the main endpoint key1 is still there
	assert.Equal(t, []string{"key1", "key4", "key2", "key3"}, resolver.GetAPIKeys())

	// The config is updated so the duplicate key is there again
	endpoints = map[string][]string{
		"example.com": {"key4", "key1", "key3"},
	}
	mockConfig.SetWithoutSource("additional_endpoints", endpoints)
	updateAdditionalEndpoints(resolver, "additional_endpoints", mockConfig, log)

	assert.Equal(t, []string{"key1", "key4", "key3"}, resolver.GetAPIKeys())
}
