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
	baseLayer := layer{
		ID: "base",
		Content: map[interface{}]interface{}{
			"api_key": "1234",
			"apm": map[interface{}]interface{}{
				"enabled":       true,
				"sampling_rate": 0.5,
			},
		},
	}
	overrideLayer := layer{
		ID: "override",
		Content: map[interface{}]interface{}{
			"apm": map[interface{}]interface{}{
				"sampling_rate": 0.7,
				"env":           "prod",
			},
		},
	}
	config, err := newConfig(baseLayer, overrideLayer)
	assert.NoError(t, err)
	serializedConfig, err := config.Marshal()
	assert.NoError(t, err)
	exprectedConfig := doNotEditDisclaimer + `
fleet_layers:
- base
- override
api_key: "1234"
apm:
  enabled: true
  env: prod
  sampling_rate: 0.7
`
	assert.Equal(t, exprectedConfig, string(serializedConfig))
}
