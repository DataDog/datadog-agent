// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"encoding/json"
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

	order := &orderConfig{
		Order: []string{"base", "override"},
	}

	config, err := newAgentConfig(order, baseLayerRaw, overrideLayerRaw)
	assert.NoError(t, err)
	expectedConfig := doNotEditDisclaimer + `
api_key: "1234"
apm:
  enabled: true
  env: prod
  sampling_rate: 0.5
fleet_layers:
- override
- base
`
	assert.Equal(t, expectedConfig, string(config.datadog))
}
