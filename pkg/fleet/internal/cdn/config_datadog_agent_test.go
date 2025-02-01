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
	baseLayer := &agentConfigLayer{
		ID: "base",
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

	overrideLayer := &agentConfigLayer{
		ID: "override",
		AgentConfig: map[string]interface{}{
			"apm": map[string]interface{}{
				"sampling_rate": 0.7,
				"env":           "prod",
			},
		},
	}
	overrideLayerRaw, err := json.Marshal(overrideLayer)
	assert.NoError(t, err)

	config, err := newAgentConfig(baseLayerRaw, overrideLayerRaw)
	assert.NoError(t, err)
	expectedConfig := doNotEditDisclaimer + `
api_key: "1234"
apm:
  enabled: true
  env: prod
  sampling_rate: 0.7
fleet_layers:
- base
- override
`
	assert.Equal(t, expectedConfig, string(config.datadog))
}

func TestAgentConfigWithIntegrations(t *testing.T) {
	layers := []*agentConfigLayer{
		{
			ID: "layer1",
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
			},
		},
		{
			ID: "layer2",
			IntegrationsConfig: []integration{
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
		},
	}
	rawLayers := make([][]byte, 0, len(layers))
	for _, layer := range layers {
		rawLayer, err := json.Marshal(layer)
		assert.NoError(t, err)
		rawLayers = append(rawLayers, rawLayer)
	}

	config, err := newAgentConfig(rawLayers...)
	assert.NoError(t, err)
	expectedAgentConfig := doNotEditDisclaimer + `
api_key: "1234"
fleet_layers:
- layer1
- layer2
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
