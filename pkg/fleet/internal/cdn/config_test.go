// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	baseLayer := &layer{
		ID: "base",
		AgentConfig: map[string]interface{}{
			"api_key": "1234",
			"apm": map[string]interface{}{
				"enabled":       true,
				"sampling_rate": 0.5,
			},
		},
	}
	overrideLayer := &layer{
		ID: "override",
		AgentConfig: map[string]interface{}{
			"apm": map[string]interface{}{
				"sampling_rate": 0.7,
				"env":           "prod",
			},
		},
	}
	config, err := newConfig(baseLayer, overrideLayer)
	assert.Nil(t, err)
	expectedConfig := doNotEditDisclaimer + `
__fleet_layers:
- base
- override
api_key: "1234"
apm:
  enabled: true
  env: prod
  sampling_rate: 0.7
`
	assert.Equal(t, expectedConfig, string(config.Datadog))
}
