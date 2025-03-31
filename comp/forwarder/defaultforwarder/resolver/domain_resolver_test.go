package resolver

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/assert"
)

func TestSingleDomainResolverDedupedKey(t *testing.T) {
	// Note key2 exists twice in the list.
	apiKeys := []utils.Endpoint{
		utils.NewEndpoint("additional_endpoints", "key1", "key2"),
		utils.NewEndpoint("multi_region_failover.api_key", "key2"),
	}

	resolver := NewSingleDomainResolver("example.com", apiKeys)

	assert.Equal(t, resolver.dedupedAPIKeys, []string{"key1", "key2"})
}
