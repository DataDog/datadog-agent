// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeRCConfigWithEmptyData(t *testing.T) {
	emptyUpdateStatus := func(_ string, _ ApplyStatus) {}

	content, err := MergeRCAgentConfig(emptyUpdateStatus, make(map[string]RawConfig))
	assert.NoError(t, err)
	assert.Equal(t, ConfigContent{}, content)
}

// rawConfig builds a RawConfig with the given JSON body for use in merge tests.
// pathName is the config ID segment (e.g. "configuration_order", "fleet_debug").
func rawConfig(pathName, body string) (string, RawConfig) {
	path := "datadog/1/AGENT_CONFIG/" + pathName + "/abc"
	return path, RawConfig{Config: []byte(body)}
}

func TestMergeRCAgentConfig_LogLevelSet(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	orderPath, orderRaw := rawConfig("configuration_order", `{"order":["fleet_debug"],"internal_order":[]}`)
	cfgPath, cfgRaw := rawConfig("fleet_debug", `{"name":"fleet_debug","config":{"log_level":"debug"}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath: orderRaw,
		cfgPath:   cfgRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "debug", content.LogLevel)
}

// TestMergeRCAgentConfig_EmptyLayerDoesNotClearLogLevel is the regression test for
// the fleet-flare log_level bug: when an Order or InternalOrder layer has no
// log_level field, it must not overwrite a valid value supplied by another layer.
func TestMergeRCAgentConfig_EmptyLayerDoesNotClearLogLevel(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	// Order: [base (no log_level), fleet_debug (log_level=debug)]
	// With reverse iteration, Order[0]="base" is processed last and previously
	// overwrote "debug" with "".
	orderPath, orderRaw := rawConfig("configuration_order", `{"order":["base","fleet_debug"],"internal_order":[]}`)
	basePath, baseRaw := rawConfig("base", `{"name":"base","config":{}}`)
	cfgPath, cfgRaw := rawConfig("fleet_debug", `{"name":"fleet_debug","config":{"log_level":"debug"}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath: orderRaw,
		basePath:  baseRaw,
		cfgPath:   cfgRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "debug", content.LogLevel)
}

// TestMergeRCAgentConfig_InternalOrderDoesNotClearLogLevel is the regression test
// for the second variant of the bug: an InternalOrder layer without log_level must
// not clear a value set by the Order loop.
func TestMergeRCAgentConfig_InternalOrderDoesNotClearLogLevel(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	orderPath, orderRaw := rawConfig("configuration_order", `{"order":["fleet_debug"],"internal_order":["internal_base"]}`)
	cfgPath, cfgRaw := rawConfig("fleet_debug", `{"name":"fleet_debug","config":{"log_level":"debug"}}`)
	internalPath, internalRaw := rawConfig("internal_base", `{"name":"internal_base","config":{}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath:    orderRaw,
		cfgPath:      cfgRaw,
		internalPath: internalRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "debug", content.LogLevel)
}

// TestMergeRCAgentConfig_ResetWhenNoLayerSetsLogLevel verifies that when all active
// configs omit log_level, the merged result is empty — which triggers the "fall back
// to previous source" path in agentConfigUpdateCallback.
func TestMergeRCAgentConfig_ResetWhenNoLayerSetsLogLevel(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	orderPath, orderRaw := rawConfig("configuration_order", `{"order":["base"],"internal_order":[]}`)
	basePath, baseRaw := rawConfig("base", `{"name":"base","config":{}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath: orderRaw,
		basePath:  baseRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "", content.LogLevel)
}

// TestMergeRCAgentConfig_OrderPriority verifies the priority semantics of the Order
// array: Order[0] is highest priority (the loop is reversed, so index 0 is written
// last and wins over all higher indices).
func TestMergeRCAgentConfig_OrderPriority(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	// Order[0]="override" (highest priority, log_level=warn) should beat Order[1]="fleet_debug" (debug).
	orderPath, orderRaw := rawConfig("configuration_order", `{"order":["override","fleet_debug"],"internal_order":[]}`)
	overridePath, overrideRaw := rawConfig("override", `{"name":"override","config":{"log_level":"warn"}}`)
	cfgPath, cfgRaw := rawConfig("fleet_debug", `{"name":"fleet_debug","config":{"log_level":"debug"}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath:    orderRaw,
		overridePath: overrideRaw,
		cfgPath:      cfgRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "warn", content.LogLevel)
}

// TestMergeRCAgentConfig_InternalOrderOverridesOrder verifies that a non-empty
// InternalOrder layer wins over a non-empty Order layer, since the InternalOrder
// loop runs after the Order loop and Order[0] is the last write.
func TestMergeRCAgentConfig_InternalOrderOverridesOrder(t *testing.T) {
	noopStatus := func(_ string, _ ApplyStatus) {}

	orderPath, orderRaw := rawConfig("configuration_order",
		`{"order":["fleet_debug"],"internal_order":["internal_override"]}`)
	cfgPath, cfgRaw := rawConfig("fleet_debug", `{"name":"fleet_debug","config":{"log_level":"debug"}}`)
	internalPath, internalRaw := rawConfig("internal_override",
		`{"name":"internal_override","config":{"log_level":"trace"}}`)

	content, err := MergeRCAgentConfig(noopStatus, map[string]RawConfig{
		orderPath:    orderRaw,
		cfgPath:      cfgRaw,
		internalPath: internalRaw,
	})
	require.NoError(t, err)
	assert.Equal(t, "trace", content.LogLevel)
}
