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
	}, nil)

	_, err := ia.getProvidedAgentConfiguration()
	assert.Error(t, err)
}

func TestGetProvidedAgentConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getProvidedAgentConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetFullAgentConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getFullAgentConfiguration()
	assert.Error(t, err)
}

func TestGetFullAgentConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getFullAgentConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetAgentFileConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getFullAgentConfiguration()
	assert.Error(t, err)
}

func TestGetAgentFileConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getAgentFileConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetAgentEnvVarConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getAgentEnvVarConfiguration()
	assert.Error(t, err)
}

func TestGetAgentEnvVarConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getAgentEnvVarConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetAgentRuntimeConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getAgentRuntimeConfiguration()
	assert.Error(t, err)
}

func TestGetAgentRuntimeConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getAgentRuntimeConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetAgentRemoteConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getAgentRemoteConfiguration()
	assert.Error(t, err)
}

func TestGetAgentRemoteConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getAgentRemoteConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetAgentCliConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getAgentCliConfiguration()
	assert.Error(t, err)
}

func TestGetAgentCliConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getAgentCliConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}
