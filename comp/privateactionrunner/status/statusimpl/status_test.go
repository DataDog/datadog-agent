// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestJSONDisabled(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(parEnabledKey, false)

	provider := statusProvider{
		config: cfg,
		isRunning: func() (bool, error) {
			return true, nil
		},
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(t, err)

	parStatus := getStatusMap(t, stats)
	assert.Equal(t, "Disabled", parStatus["status"])
	assert.Equal(t, false, parStatus["enabled"])
}

func TestJSONEnabledRunning(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(parEnabledKey, true)
	cfg.SetWithoutSource(parSelfEnrollKey, true)
	cfg.SetWithoutSource(parURNKey, "urn:dd:apps:on-prem-runner:us5:12345:runner-abc")

	provider := statusProvider{
		config: cfg,
		isRunning: func() (bool, error) {
			return true, nil
		},
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(t, err)

	parStatus := getStatusMap(t, stats)
	assert.Equal(t, "Running", parStatus["status"])
	assert.Equal(t, true, parStatus["enabled"])
	assert.Equal(t, true, parStatus["selfEnroll"])
	assert.Equal(t, true, parStatus["urnConfigured"])
	assert.NotEmpty(t, parStatus["runnerVersion"])
}

func TestJSONEnabledNotRunning(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(parEnabledKey, true)

	provider := statusProvider{
		config: cfg,
		isRunning: func() (bool, error) {
			return false, nil
		},
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(t, err)

	parStatus := getStatusMap(t, stats)
	assert.Equal(t, "Not running or unreachable", parStatus["status"])
	assert.Equal(t, true, parStatus["enabled"])
}

func TestJSONError(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(parEnabledKey, true)

	provider := statusProvider{
		config: cfg,
		isRunning: func() (bool, error) {
			return false, errors.New("process listing failed")
		},
	}

	stats := make(map[string]interface{})
	err := provider.JSON(false, stats)
	require.NoError(t, err)

	parStatus := getStatusMap(t, stats)
	assert.Equal(t, "Unknown", parStatus["status"])
	assert.Equal(t, "process listing failed", parStatus["error"])
}

func TestText(t *testing.T) {
	cfg := config.NewMock(t)
	cfg.SetWithoutSource(parEnabledKey, true)

	provider := statusProvider{
		config: cfg,
		isRunning: func() (bool, error) {
			return true, nil
		},
	}

	buffer := new(bytes.Buffer)
	err := provider.Text(false, buffer)
	require.NoError(t, err)

	assert.Contains(t, buffer.String(), "Status: Running")
	assert.Contains(t, buffer.String(), "Enabled: True")
}

func TestIsPrivateActionRunnerProcess(t *testing.T) {
	assert.True(t, isPrivateActionRunnerProcess("privateactionrunner"))
	assert.True(t, isPrivateActionRunnerProcess("privateactionrunner.exe"))
	assert.True(t, isPrivateActionRunnerProcess("datadog-private-action-runner"))
	assert.True(t, isPrivateActionRunnerProcess("datadog-agent-action"))
	assert.False(t, isPrivateActionRunnerProcess("trace-agent"))
}

func getStatusMap(t *testing.T, stats map[string]interface{}) map[string]interface{} {
	t.Helper()
	rawStatus, ok := stats["privateActionRunnerStatus"]
	require.True(t, ok)

	parStatus, ok := rawStatus.(map[string]interface{})
	require.True(t, ok)
	return parStatus
}
