// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"testing"

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

	assert.Equal(t, resolver.dedupedAPIKeys, []string{"key1", "key2"})
}

func TestSingleDomainResolverSetApiKeysSimple(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "key1", "key2"),
		utils.NewAPIKeys("multi_region_failover.api_key", "key2"),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	removed, added := resolver.SetAPIKeys([]string{"key1", "key3"})

	assert.Equal(t, []string{"key2"}, removed)
	assert.Equal(t, []string{"key3"}, added)
}

func TestSingleDomainResolverSetApiKeysMany(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", "key1", "key2", "key3", "key4", "key5", "key6"),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	removed, added := resolver.SetAPIKeys([]string{"key3", "lock2", "key1", "lock4", "key5", "key6"})

	assert.Equal(t, []string{"key2", "key4"}, removed)
	assert.Equal(t, []string{"lock2", "lock4"}, added)
}
