package resolver

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/stretchr/testify/assert"
)

func TestSingleDomainResolverDedupedKey(t *testing.T) {
	mockConfig := config.NewMock(t)
	mockConfig.SetWithoutSource("additional_endpoints", map[string][]string{
		"example1.com": []string{"key1", "key2"},
	})
	log := logmock.New(t)

	// Note key2 exists twice in the list.
	apiKeys := []utils.Endpoint{
		utils.Endpoint{
			ConfigSettingPath: "additional_endpoints",
			ApiKeys:           []string{"key1", "key2"},
		},
		utils.Endpoint{
			ConfigSettingPath: "multi_region_failover.api_key",
			ApiKeys:           []string{"key2"},
		},
	}

	resolver := NewSingleDomainResolver(mockConfig, log, "example.com", apiKeys)

	assert.Equal(t, resolver.dedupedApiKeys, []string{"key1", "key2"})
}
