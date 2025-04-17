// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/stretchr/testify/assert"
)

// Makes a key exactly 32 chars long
func makeKey(suffix string) string {
	return strings.Repeat("0", 32-len(suffix)) + suffix
}

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

func TestSingleDomainResolverSetApiKeysSimple(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", makeKey("key1"), makeKey("key2")),
		utils.NewAPIKeys("multi_region_failover.api_key", makeKey("key2")),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	removed, added := resolver.SetAPIKeys([]string{makeKey("key1"), makeKey("key3")})

	assert.Equal(t, []string{scrubber.HideKeyExceptLastFiveChars(makeKey("key2"))}, removed)
	assert.Equal(t, []string{scrubber.HideKeyExceptLastFiveChars(makeKey("key3"))}, added)
}

func TestSingleDomainResolverSetApiKeysMany(t *testing.T) {
	apiKeys := []utils.APIKeys{
		utils.NewAPIKeys("additional_endpoints", makeKey("key1"), makeKey("key2"), makeKey("key3"), makeKey("key4"), makeKey("key5"), makeKey("key6")),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	removed, added := resolver.SetAPIKeys([]string{makeKey("key3"), makeKey("lock2"), makeKey("key1"), makeKey("lock4"), makeKey("key5"), makeKey("key6")})

	assert.Equal(t, []string{
		scrubber.HideKeyExceptLastFiveChars(makeKey("key2")),
		scrubber.HideKeyExceptLastFiveChars(makeKey("key4")),
	}, removed)

	assert.Equal(t, []string{
		scrubber.HideKeyExceptLastFiveChars(makeKey("lock2")),
		scrubber.HideKeyExceptLastFiveChars(makeKey("lock4")),
	}, added)
}
