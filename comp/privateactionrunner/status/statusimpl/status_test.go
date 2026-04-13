// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestStatusEnabled(t *testing.T) {
	configComponent := config.NewMock(t)
	configComponent.SetWithoutSource("private_action_runner.enabled", true)
	configComponent.SetWithoutSource("private_action_runner.urn", "urn:datadog:action-runner:abcdef123456")
	configComponent.SetWithoutSource("private_action_runner.self_enroll", true)
	configComponent.SetWithoutSource("private_action_runner.actions_allowlist", []string{"com.datadoghq.http.request"})
	configComponent.SetWithoutSource("private_action_runner.default_actions_enabled", true)

	provider := statusProvider{config: configComponent}

	t.Run("JSON", func(t *testing.T) {
		stats := make(map[string]interface{})
		err := provider.JSON(false, stats)
		require.NoError(t, err)

		parStats, ok := stats["privateActionRunnerStatus"].(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, true, parStats["Enabled"])
		assert.Equal(t, true, parStats["SelfEnroll"])
		assert.Equal(t, true, parStats["DefaultActionsEnabled"])
		assert.Equal(t, "urn:datadog:action-runner:abcdef123456", parStats["URN"])
		assert.Contains(t, parStats["ActionsAllowlist"], "com.datadoghq.http.request")
		assert.Contains(t, parStats["ActionsAllowlist"], "com.datadoghq.kubernetes.core.getPod")
	})

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := provider.Text(false, b)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Enabled")
		assert.Contains(t, output, "123456")
		assert.Contains(t, output, "Default Actions Enabled: true")
		assert.Contains(t, output, "com.datadoghq.kubernetes.core.getPod")
	})

	t.Run("HTML", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := provider.HTML(false, b)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Enabled")
		assert.Contains(t, output, "123456")
		assert.Contains(t, output, "Default Actions Enabled: true")
		assert.Contains(t, output, "com.datadoghq.kubernetes.core.getPod")
	})
}

func TestStatusDisabled(t *testing.T) {
	configComponent := config.NewMock(t)
	configComponent.SetWithoutSource("private_action_runner.enabled", false)

	provider := statusProvider{config: configComponent}

	t.Run("JSON", func(t *testing.T) {
		stats := make(map[string]interface{})
		err := provider.JSON(false, stats)
		require.NoError(t, err)

		parStats, ok := stats["privateActionRunnerStatus"].(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, false, parStats["Enabled"])
		assert.Nil(t, parStats["URN"])
	})

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		err := provider.Text(false, b)
		require.NoError(t, err)

		output := b.String()
		assert.Contains(t, output, "Disabled")
		assert.NotContains(t, output, "URN")
	})
}

func TestProviderNameAndSection(t *testing.T) {
	provider := statusProvider{}
	assert.Equal(t, "Private Action Runner", provider.Name())
	assert.Equal(t, "Private Action Runner", provider.Section())
}
