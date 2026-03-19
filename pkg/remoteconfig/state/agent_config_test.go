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

// TestMergeRCConfigOrderArrivesBeforeConfig reproduces the race where the
// configuration_order file arrives in one poll but the referenced config file
// has not been stored in state yet. The previous behaviour silently returned an
// empty ConfigContent (log_level ""), causing a spurious log and an early-return
// that never acknowledged the configs.  The fix returns an explicit error so the
// callback treats it as a transient failure and the RC backend retries.
func TestMergeRCConfigOrderArrivesBeforeConfig(t *testing.T) {
	emptyUpdateStatus := func(_ string, _ ApplyStatus) {}

	// Only the order file is present; the referenced config is missing.
	updates := map[string]RawConfig{
		"datadog/2/AGENT_CONFIG/configuration_order/configuration_order": {
			Config: []byte(`{"order":["flare-log-level-abc123"],"internal_order":[]}`),
		},
	}

	_, err := MergeRCAgentConfig(emptyUpdateStatus, updates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flare-log-level-abc123")
}

// TestMergeRCConfigBothArriveTogether verifies that when both the order file and
// the referenced config are present in the same update the log level is returned.
func TestMergeRCConfigBothArriveTogether(t *testing.T) {
	emptyUpdateStatus := func(_ string, _ ApplyStatus) {}

	updates := map[string]RawConfig{
		"datadog/2/AGENT_CONFIG/configuration_order/configuration_order": {
			Config: []byte(`{"order":["flare-log-level-abc123"],"internal_order":[]}`),
		},
		"datadog/2/AGENT_CONFIG/flare-log-level-abc123/flare-log-level-abc123": {
			Config: []byte(`{"name":"flare-log-level-abc123","config":{"log_level":"debug"}}`),
		},
	}

	content, err := MergeRCAgentConfig(emptyUpdateStatus, updates)
	require.NoError(t, err)
	assert.Equal(t, "debug", content.LogLevel)
}
