package common

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	apiKey string
)

// CachePrefix is the common root to use to prefix all the cache
// keys for any metadata value
const CachePrefix = "metadata"

// GetPayload fills and return the common metadata payload
func GetPayload() *Payload {
	return &Payload{
		AgentVersion: version.AgentVersion,
		APIKey:       getAPIKey(),
	}
}

func getAPIKey() string {
	if apiKey == "" {
		apiKey = strings.Split(config.Datadog.GetString("api_key"), ",")[0]
	}

	return apiKey
}
