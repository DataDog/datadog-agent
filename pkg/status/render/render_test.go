// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package render

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatStatus(t *testing.T) {
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()
	agentJSON, err := os.ReadFile("fixtures/agent_status.json")
	require.NoError(t, err)
	agentText, err := os.ReadFile("fixtures/agent_status.text")
	require.NoError(t, err)
	const statusRenderErrors = "Status render errors"

	t.Run("render errors", func(t *testing.T) {
		actual, err := FormatStatus([]byte{})
		require.NoError(t, err)
		assert.Contains(t, actual, statusRenderErrors)
	})

	t.Run("no render errors", func(t *testing.T) {
		actual, err := FormatStatus(agentJSON)
		require.NoError(t, err)

		// We replace windows line break by linux so the tests pass on every OS
		result := strings.Replace(string(agentText), "\r\n", "\n", -1)
		actual = strings.Replace(actual, "\r\n", "\n", -1)

		assert.Equal(t, actual, result)
		assert.NotContains(t, actual, statusRenderErrors)
	})
}

func TestFormatDCAStatus(t *testing.T) {
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()
	agentJSON, err := os.ReadFile("fixtures/cluster_agent_status.json")
	require.NoError(t, err)
	agentText, err := os.ReadFile("fixtures/cluster_agent_status.text")
	require.NoError(t, err)
	const statusRenderErrors = "Status render errors"

	t.Run("render errors", func(t *testing.T) {
		actual, err := FormatDCAStatus([]byte{})
		require.NoError(t, err)
		assert.Contains(t, actual, statusRenderErrors)
	})

	t.Run("no render errors", func(t *testing.T) {
		actual, err := FormatDCAStatus(agentJSON)
		require.NoError(t, err)

		// We replace windows line break by linux so the tests pass on every OS
		result := strings.Replace(string(agentText), "\r\n", "\n", -1)
		actual = strings.Replace(actual, "\r\n", "\n", -1)

		assert.Equal(t, actual, result)
		assert.NotContains(t, actual, statusRenderErrors)
	})
}

func TestFormatSecurityAgentStatus(t *testing.T) {
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()
	agentJSON, err := os.ReadFile("fixtures/security_agent_status.json")
	require.NoError(t, err)
	agentText, err := os.ReadFile("fixtures/security_agent_status.text")
	require.NoError(t, err)
	const statusRenderErrors = "Status render errors"

	t.Run("render errors", func(t *testing.T) {
		actual, err := FormatSecurityAgentStatus([]byte{})
		require.NoError(t, err)
		assert.Contains(t, actual, statusRenderErrors)
	})

	t.Run("no render errors", func(t *testing.T) {
		actual, err := FormatSecurityAgentStatus(agentJSON)
		require.NoError(t, err)

		// We replace windows line break by linux so the tests pass on every OS
		result := strings.Replace(string(agentText), "\r\n", "\n", -1)
		actual = strings.Replace(actual, "\r\n", "\n", -1)

		assert.Equal(t, actual, result)
		assert.NotContains(t, actual, statusRenderErrors)
	})
}
