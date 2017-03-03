package common

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/stretchr/testify/assert"
)

func TestGetPayload(t *testing.T) {
	apiKey = "foo"
	p := GetPayload()
	assert.Equal(t, p.APIKey, "foo")
	assert.Equal(t, p.AgentVersion, version.AgentVersion)
	apiKey = ""
}

func TestGetAPIKey(t *testing.T) {
	config.Datadog.SetDefault("api_key", "bar,baz")
	assert.Equal(t, "bar", getAPIKey())
	assert.Equal(t, "bar", apiKey)
	apiKey = "foo"
	assert.Equal(t, "foo", getAPIKey())
}
