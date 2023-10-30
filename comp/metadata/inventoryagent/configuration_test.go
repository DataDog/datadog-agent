// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProvidedAgentConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	})

	_, err := ia.getProvidedAgentConfiguration()
	assert.Error(t, err)
}

func TestGetProvidedAgentConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	})

	data, err := ia.getProvidedAgentConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetFullAgentConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	})

	_, err := ia.getFullAgentConfiguration()
	assert.Error(t, err)
}

func TestGetFullAgentConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	})

	data, err := ia.getFullAgentConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}
