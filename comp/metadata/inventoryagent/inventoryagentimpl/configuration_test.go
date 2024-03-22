// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProvidedConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getProvidedConfiguration()
	assert.Error(t, err)
}

func TestGetProvidedConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getProvidedConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetFullConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getFullConfiguration()
	assert.Error(t, err)
}

func TestGetFullConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getFullConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetFileConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getFullConfiguration()
	assert.Error(t, err)
}

func TestGetFileConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getFileConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetEnvVarConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getEnvVarConfiguration()
	assert.Error(t, err)
}

func TestGetEnvVarConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getEnvVarConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetRuntimeConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getRuntimeConfiguration()
	assert.Error(t, err)
}

func TestGetRuntimeConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getRuntimeConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetRemoteConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getRemoteConfiguration()
	assert.Error(t, err)
}

func TestGetRemoteConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getRemoteConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetCliConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getCliConfiguration()
	assert.Error(t, err)
}

func TestGetCliConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getCliConfiguration()
	assert.NoError(t, err)
	assert.NotEmpty(t, data)
}

func TestGetSourceLocalConfigurationDisable(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": false,
	}, nil)

	_, err := ia.getSourceLocalConfiguration()
	assert.Error(t, err)
}

func TestGetSourceLocalConfiguration(t *testing.T) {
	ia := getTestInventoryPayload(t, map[string]any{
		"inventories_configuration_enabled": true,
	}, nil)

	data, err := ia.getSourceLocalConfiguration()
	assert.NoError(t, err)
	t.Log(data)
	assert.NotEmpty(t, data)
}
