package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentConfig(t *testing.T) {
	fileName := "testdata/config.yaml"
	c, err := AgentConfig([]string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
}
