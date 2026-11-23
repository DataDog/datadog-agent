// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStatusConfig struct {
	Component
	configFile       string
	extraConfigFiles []string
	warnings         []string
}

func (f fakeStatusConfig) ConfigFileUsed() string {
	return f.configFile
}

func (f fakeStatusConfig) ExtraConfigFilesUsed() []string {
	return f.extraConfigFiles
}

func (f fakeStatusConfig) Warnings() []string {
	return f.warnings
}

func TestStatusProvider(t *testing.T) {
	warnings := []string{
		"invalid value for 'log_level', reverting to default",
		"invalid value for 'cmd_port', reverting to default",
	}
	provider := statusProvider{config: fakeStatusConfig{
		Component:        NewMock(t),
		configFile:       "/etc/datadog-agent/datadog.yaml",
		extraConfigFiles: []string{"/etc/datadog-agent/extra.yaml"},
		warnings:         warnings,
	}}

	t.Run("JSON", func(t *testing.T) {
		stats := map[string]interface{}{}
		require.NoError(t, provider.JSON(false, stats))
		assert.Equal(t, "/etc/datadog-agent/datadog.yaml", stats["configFile"])
		assert.Equal(t, []string{"/etc/datadog-agent/extra.yaml"}, stats["extraConfigFiles"])
		assert.Equal(t, warnings, stats["configWarnings"])
	})

	t.Run("Text", func(t *testing.T) {
		b := new(bytes.Buffer)
		require.NoError(t, provider.Text(false, b))
		assert.Contains(t, b.String(), "Configuration file: /etc/datadog-agent/datadog.yaml")
		assert.Contains(t, b.String(), "- /etc/datadog-agent/extra.yaml")
		for _, w := range warnings {
			assert.Contains(t, b.String(), "- "+w)
		}
	})

	t.Run("HTML", func(t *testing.T) {
		b := new(bytes.Buffer)
		require.NoError(t, provider.HTML(false, b))
		assert.Contains(t, b.String(), "Configuration file: /etc/datadog-agent/datadog.yaml")
		assert.Contains(t, b.String(), "- /etc/datadog-agent/extra.yaml")
		assert.Contains(t, b.String(), "invalid value for &#39;log_level&#39;, reverting to default")
		assert.Contains(t, b.String(), "invalid value for &#39;cmd_port&#39;, reverting to default")
	})
}

func TestStatusProviderEmpty(t *testing.T) {
	provider := statusProvider{config: fakeStatusConfig{Component: NewMock(t)}}

	b := new(bytes.Buffer)
	require.NoError(t, provider.Text(false, b))
	assert.Contains(t, b.String(), "No configuration file used")
	assert.NotContains(t, b.String(), "Extra configuration files")
	assert.Contains(t, b.String(), "No configuration warnings")

	b.Reset()
	require.NoError(t, provider.HTML(false, b))
	assert.Contains(t, b.String(), "No configuration file used")
	assert.NotContains(t, b.String(), "Extra configuration files")
	assert.Contains(t, b.String(), "No configuration warnings")
}
