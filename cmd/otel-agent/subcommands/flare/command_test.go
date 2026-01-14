// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newGlobalParamsTest(t *testing.T) *subcommands.GlobalParams {
	tmpDir := t.TempDir()

	// Create a simple OTel configuration file
	configPath := path.Join(tmpDir, "otel-config.yaml")
	configContent := `receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

exporters:
  datadog:
    api:
      key: test-api-key

extensions:
  health_check:
    endpoint: localhost:13133
  pprof:
    endpoint: localhost:1777
  zpages:
    endpoint: localhost:55679

service:
  extensions: [health_check, pprof, zpages]
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [datadog]
    metrics:
      receivers: [otlp]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      exporters: [datadog]
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	return &subcommands.GlobalParams{
		ConfPaths: []string{"file:" + configPath},
	}
}

func TestCreateOTelFlare(t *testing.T) {
	globalParams := newGlobalParamsTest(t)

	// Create the flare
	flarePath, err := createOTelFlare(globalParams)
	require.NoError(t, err, "createOTelFlare should succeed")
	require.NotEmpty(t, flarePath, "flare path should not be empty")

	// Verify the file exists
	_, err = os.Stat(flarePath)
	require.NoError(t, err, "flare file should exist")

	// Clean up
	defer os.Remove(flarePath)

	// Verify the zip file contains expected files
	zipReader, err := zip.OpenReader(flarePath)
	require.NoError(t, err, "should be able to open zip file")
	defer zipReader.Close()

	expectedFiles := map[string]bool{
		"otel-response.json":         false,
		"build-info.txt":             false,
		"config/env-config.yaml":     false,
		"config/runtime-config.yaml": false,
		"environment.json":           false,
		"debug-sources.json":         false,
	}

	for _, file := range zipReader.File {
		if _, exists := expectedFiles[file.Name]; exists {
			expectedFiles[file.Name] = true
		}
	}

	// Verify all expected files are present
	for fileName, found := range expectedFiles {
		assert.True(t, found, "Expected file %s not found in flare archive", fileName)
	}
}

func TestCollectOTelData(t *testing.T) {
	globalParams := newGlobalParamsTest(t)

	// Collect OTel data
	data, err := collectOTelData(globalParams)
	require.NoError(t, err, "collectOTelData should succeed")
	require.NotNil(t, data, "collected data should not be nil")

	// Verify basic response fields
	assert.Equal(t, "otel-agent", data.AgentCommand)
	assert.Equal(t, "Datadog OTel Agent", data.AgentDesc)
	assert.NotEmpty(t, data.AgentVersion, "agent version should not be empty")

	// Verify configs are populated
	assert.NotEmpty(t, data.RuntimeConfig, "runtime config should not be empty")
	assert.NotEmpty(t, data.EnvConfig, "env config should not be empty")

	// Verify environment is populated
	assert.NotEmpty(t, data.Environment, "environment should not be empty")

	// Verify debug sources are discovered
	assert.NotNil(t, data.Sources, "debug sources should not be nil")

	// Check for expected debug extensions
	expectedExtensions := []string{"health_check", "pprof", "zpages"}
	for _, ext := range expectedExtensions {
		source, exists := data.Sources[ext]
		assert.True(t, exists, "Expected debug extension %s not found", ext)
		if exists {
			assert.NotEmpty(t, source.URLs, "Debug extension %s should have URLs", ext)
		}
	}
}

func TestDiscoverDebugSources(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test configuration with debug extensions
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `extensions:
  health_check:
    endpoint: localhost:13133
  pprof:
    endpoint: localhost:1777
  zpages:
    endpoint: localhost:55679
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Discover debug sources
	sources := discoverDebugSources(context.TODO(), []string{"file:" + configPath})

	// Verify all three extensions are discovered
	assert.Contains(t, sources, "health_check", "health_check should be discovered")
	assert.Contains(t, sources, "pprof", "pprof should be discovered")
	assert.Contains(t, sources, "zpages", "zpages should be discovered")

	// Verify health_check URLs
	if healthCheck, ok := sources["health_check"]; ok {
		assert.Len(t, healthCheck.URLs, 1, "health_check should have 1 URL")
		assert.Contains(t, healthCheck.URLs[0], "http://localhost:13133")
	}

	// Verify pprof URLs
	if pprof, ok := sources["pprof"]; ok {
		assert.Len(t, pprof.URLs, 3, "pprof should have 3 URLs")
		assert.Contains(t, pprof.URLs[0], "http://localhost:1777/debug/pprof/heap")
		assert.Contains(t, pprof.URLs[1], "http://localhost:1777/debug/pprof/allocs")
		assert.Contains(t, pprof.URLs[2], "http://localhost:1777/debug/pprof/profile")
	}

	// Verify zpages URLs
	if zpages, ok := sources["zpages"]; ok {
		assert.Len(t, zpages.URLs, 5, "zpages should have 5 URLs")
		assert.Contains(t, zpages.URLs[0], "http://localhost:55679/debug/servicez")
		assert.Contains(t, zpages.URLs[1], "http://localhost:55679/debug/pipelinez")
		assert.Contains(t, zpages.URLs[2], "http://localhost:55679/debug/extensionz")
		assert.Contains(t, zpages.URLs[3], "http://localhost:55679/debug/featurez")
		assert.Contains(t, zpages.URLs[4], "http://localhost:55679/debug/tracez")
	}
}

func TestExtractExtensionType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple extension name",
			input:    "health_check",
			expected: "health_check",
		},
		{
			name:     "extension with instance name",
			input:    "pprof/dd-autoconfigured",
			expected: "pprof",
		},
		{
			name:     "extension with multiple slashes",
			input:    "zpages/custom/instance",
			expected: "zpages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExtensionType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFlareCommand(t *testing.T) {
	globalConfGetter := func() *subcommands.GlobalParams {
		return newGlobalParamsTest(t)
	}
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{MakeCommand(globalConfGetter)},
		[]string{"flare", "--send"},
		makeFlare,
		func() {},
	)
}
