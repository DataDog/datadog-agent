// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestStatusEnabled(t *testing.T) {
	flavor.SetFlavor(flavor.ClusterAgent)
	defer flavor.SetFlavor(flavor.DefaultAgent)

	configComponent := config.NewMock(t)
	configComponent.SetInTest("private_action_runner.enabled", true)
	configComponent.SetInTest("private_action_runner.urn", "urn:datadog:action-runner:abcdef123456")
	configComponent.SetInTest("private_action_runner.self_enroll", true)
	configComponent.SetInTest("private_action_runner.actions_allowlist", []string{"com.datadoghq.http.request"})
	configComponent.SetInTest("private_action_runner.default_actions_enabled", true)

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
	configComponent.SetInTest("private_action_runner.enabled", false)

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

type phoneHomeStub struct{ state *phonehome.State }

func (s phoneHomeStub) Status() *phonehome.State { return s.state }

func TestPhoneHomeStartupStatus(t *testing.T) {
	for _, state := range []phonehome.State{
		{State: "waiting", Reason: "missing_enrollment_scope", NextCheck: "2026-10-23T10:01:00Z"},
		{State: "blocked", Reason: "launch_outcome_unknown_requires_review"},
		{State: "handed_off", Reason: "supervisor_start_acknowledged"},
	} {
		t.Run(state.State, func(t *testing.T) {
			provider := statusProvider{config: config.NewMock(t), phoneHome: phoneHomeStub{state: &state}}
			stats := make(map[string]interface{})
			require.NoError(t, provider.JSON(false, stats))
			encoded, err := json.Marshal(stats["privateActionRunnerStatus"].(map[string]interface{})["PhoneHome"])
			require.NoError(t, err)
			var fields map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &fields))
			expected := map[string]interface{}{"state": state.State, "reason": state.Reason}
			if state.NextCheck != "" {
				expected["next_check"] = state.NextCheck
			}
			require.Equal(t, expected, fields, "no RC or recovery diagnostics in JSON")
			var text, html bytes.Buffer
			require.NoError(t, provider.Text(false, &text))
			require.NoError(t, provider.HTML(false, &html))
			for _, output := range []string{text.String(), html.String()} {
				require.Contains(t, output, "Startup gate: "+state.State+" ("+state.Reason+")")
				require.NotContains(t, output, "RC prerequisite")
				require.NotContains(t, output, "Enrollment outcome")
				require.NotContains(t, output, "retention")
				if state.NextCheck != "" {
					require.Contains(t, output, "Next gate check: "+state.NextCheck)
				} else {
					require.NotContains(t, output, "Next gate check:")
				}
			}
		})
	}
	for _, source := range []phonehome.Component{nil, phoneHomeStub{}} {
		provider := statusProvider{config: config.NewMock(t), phoneHome: source}
		stats := make(map[string]interface{})
		require.NoError(t, provider.JSON(false, stats))
		require.NotContains(t, stats["privateActionRunnerStatus"], "PhoneHome")
	}
}

func TestProviderNameAndSection(t *testing.T) {
	provider := statusProvider{}
	assert.Equal(t, "Private Action Runner", provider.Name())
	assert.Equal(t, "Private Action Runner", provider.Section())
}
