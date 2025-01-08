// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentConfig(t *testing.T) {
	baseLayer := &agentConfigRaw{
		AgentConfig: map[string]interface{}{
			"api_key": "1234",
			"apm": map[string]interface{}{
				"enabled":       true,
				"sampling_rate": 0.5,
			},
		},
	}
	baseLayerRaw, err := json.Marshal(baseLayer)
	assert.NoError(t, err)

	configLayer := &configLayer{
		ID:     "policy",
		Config: baseLayerRaw,
	}

	configLayerRaw, err := json.Marshal(configLayer)
	assert.NoError(t, err)

	config, err := newAgentConfig(configLayerRaw)
	assert.NoError(t, err)
	expectedConfig := doNotEditDisclaimer + `
api_key: "1234"
apm:
  enabled: true
  sampling_rate: 0.5
fleet_layers:
- policy
`
	assert.Equal(t, expectedConfig, string(config.datadog))
}

func TestAgentConfigWithIntegrations(t *testing.T) {
	layer := &agentConfigRaw{
		AgentConfig: map[string]interface{}{
			"api_key": "1234",
		},
		IntegrationsConfig: []integration{
			{
				Type: "apache",
				Instance: map[string]interface{}{
					"status_url": "http://localhost:1234/server-status",
				},
				Init: map[string]interface{}{
					"apache_status_url": "http://localhost:1234/server-status",
				},
			},
			{
				Type: "apache",
				Instance: map[string]interface{}{
					"status_url": "http://localhost:5678/server-status",
				},
				Init: map[string]interface{}{
					"apache_status_url": "http://localhost:5678/server-status",
				},
			},
		},
	}
	rawLayer, err := json.Marshal(layer)
	assert.NoError(t, err)

	configLayer := &configLayer{
		ID:     "policy",
		Config: rawLayer,
	}

	configLayerRaw, err := json.Marshal(configLayer)
	assert.NoError(t, err)

	config, err := newAgentConfig(configLayerRaw)
	assert.NoError(t, err)
	expectedAgentConfig := doNotEditDisclaimer + `
api_key: "1234"
fleet_layers:
- policy
`
	assert.Equal(t, expectedAgentConfig, string(config.datadog))

	expectedIntegrationsConfig := []integration{
		{
			Type: "apache",
			Instance: map[string]interface{}{
				"status_url": "http://localhost:1234/server-status",
			},
			Init: map[string]interface{}{
				"apache_status_url": "http://localhost:1234/server-status",
			},
		},
		{
			Type: "apache",
			Instance: map[string]interface{}{
				"status_url": "http://localhost:5678/server-status",
			},
			Init: map[string]interface{}{
				"apache_status_url": "http://localhost:5678/server-status",
			},
		},
	}
	assert.Equal(t, expectedIntegrationsConfig, config.integrations)

	tmpDir := t.TempDir()
	err = config.writeIntegration(tmpDir, config.integrations[0], os.Getuid(), os.Getgid())
	assert.NoError(t, err)
	err = config.writeIntegration(tmpDir, config.integrations[1], os.Getuid(), os.Getgid())
	assert.NoError(t, err)

	// Check that the integrations are written to the correct folder
	files, err := os.ReadDir(tmpDir + "/conf.d/apache.d")
	assert.NoError(t, err)
	assert.Len(t, files, 2)
}
