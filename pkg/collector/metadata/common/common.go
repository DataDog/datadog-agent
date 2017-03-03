package common

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	apiKey string
)

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
