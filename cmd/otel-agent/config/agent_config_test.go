package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentConfig(t *testing.T) {
	fileName := "testdata/config.yaml"
	c, err := NewConfigComponent(context.Background(), []string{fileName})
	if err != nil {
		t.Errorf("Failed to load agent config: %v", err)
	}
	assert.Equal(t, "DATADOG_API_KEY", c.Get("api_key"))
	assert.Equal(t, "datadoghq.com", c.Get("site"))
	assert.Equal(t, "debug", c.Get("log_level"))
}
