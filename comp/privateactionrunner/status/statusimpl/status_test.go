// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package statusimpl

import (
	"bytes"
	"testing"
	"time"

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

func TestPhoneHomeStatusExcludesRCDiagnostics(t *testing.T) {
	cfg := config.NewMock(t)
	provider := statusProvider{config: cfg, phoneHome: phoneHomeStub{state: &phonehome.State{
		State: "waiting", Reason: "missing_enrollment_scope", RCReady: true, RCReason: "rc_ready",
	}}}
	stats := make(map[string]interface{})
	require.NoError(t, provider.JSON(false, stats))
	parStats := stats["privateActionRunnerStatus"].(map[string]interface{})
	ph := parStats["PhoneHome"].(map[string]interface{})
	require.Equal(t, "waiting", ph["State"])
	require.NotContains(t, ph, "RCReady")
	require.NotContains(t, ph, "RCReason")
	var text, html bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	require.NoError(t, provider.HTML(false, &html))
	for _, output := range []string{text.String(), html.String()} {
		require.Contains(t, output, "waiting (missing_enrollment_scope)")
		require.NotContains(t, output, "RC prerequisite")
	}
	for _, source := range []phonehome.Component{nil, phoneHomeStub{}} {
		provider.phoneHome = source
		stats = make(map[string]interface{})
		require.NoError(t, provider.JSON(false, stats))
		require.NotContains(t, stats["privateActionRunnerStatus"], "PhoneHome")
	}
}

func TestPhoneHomeStatusTemplates(t *testing.T) {
	provider := statusProvider{config: config.NewMock(t), phoneHome: phoneHomeStub{state: &phonehome.State{
		State: "waiting", Reason: "missing_enrollment_scope", NextRetry: time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}}}
	var text, html bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	require.NoError(t, provider.HTML(false, &html))
	for _, output := range []string{text.String(), html.String()} {
		require.Contains(t, output, "waiting (missing_enrollment_scope)")
		require.Contains(t, output, "Next reconciliation:")
	}
}

func TestPhoneHomeRecoveryStatusTemplates(t *testing.T) {
	provider := statusProvider{config: config.NewMock(t), phoneHome: phoneHomeStub{state: &phonehome.State{
		State: "blocked", Reason: "quota", ScopeChecked: true, ScopeReady: true, EnrollmentOutcome: "quota", RetrySafe: true, NextDiscovery: "2026-10-06T10:01:00Z", NextEnrollmentAttempt: "2026-10-06T10:05:00Z",
	}}}
	var text, html bytes.Buffer
	require.NoError(t, provider.Text(false, &text))
	require.NoError(t, provider.HTML(false, &html))
	for _, output := range []string{text.String(), html.String()} {
		require.Contains(t, output, "Scope checked: true; scope ready: true")
		require.Contains(t, output, "Enrollment outcome: quota")
		require.Contains(t, output, "Safe to retry: true; reconciliation required: false")
		require.Contains(t, output, "Next discovery: 2026-10-06T10:01:00Z")
		require.Contains(t, output, "Next enrollment attempt: 2026-10-06T10:05:00Z")
	}
}

func TestProviderNameAndSection(t *testing.T) {
	provider := statusProvider{}
	assert.Equal(t, "Private Action Runner", provider.Name())
	assert.Equal(t, "Private Action Runner", provider.Section())
}
