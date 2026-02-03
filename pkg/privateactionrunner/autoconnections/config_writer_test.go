// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestEnsureScriptBundleConfig(t *testing.T) {
	// Create temporary directory to simulate /etc/privateactionrunner
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "script-config.yaml")

	writer := ConfigWriter{BaseDir: tempDir}

	created, err := writer.EnsureScriptBundleConfig()
	require.NoError(t, err)
	assert.True(t, created, "File should be created on first call")

	// Verify file exists
	_, err = os.Stat(configPath)
	require.NoError(t, err, "Config file should exist")

	// Idempotency
	created, err = writer.EnsureScriptBundleConfig()
	require.NoError(t, err)
	assert.False(t, created, "File should not be created on second call")

	// Verify content
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var config map[string]interface{}
	err = yaml.Unmarshal(content, &config)
	require.NoError(t, err)

	assert.Equal(t, "script-credentials-v1", config["schemaId"])
	assert.NotNil(t, config["runPredefinedScript"])

	// Verify sample scripts are present
	scripts, ok := config["runPredefinedScript"].(map[string]interface{})
	require.True(t, ok, "runPredefinedScript should be a map")
	assert.Contains(t, scripts, "echo")
	assert.Contains(t, scripts, "echo-parametrized")

	// Verify permissions
	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(ConfigFilePermissions), info.Mode().Perm())

	// Verify directory permissions
	dirInfo, err := os.Stat(tempDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(ConfigDirPermissions), dirInfo.Mode().Perm())
}

func TestEnsureScriptBundleConfig_ErrorCases(t *testing.T) {
	// Test with readonly parent directory
	tempDir := t.TempDir()

	// Make directory readonly
	err := os.Chmod(tempDir, 0555)
	require.NoError(t, err)
	defer os.Chmod(tempDir, 0755)

	writer := ConfigWriter{BaseDir: tempDir}
	created, err := writer.EnsureScriptBundleConfig()
	assert.Error(t, err)
	assert.False(t, created)
	assert.Contains(t, err.Error(), "failed to write config file")
}

func TestNewDefaultConfigWriter(t *testing.T) {
	writer := NewDefaultConfigWriter()
	assert.Equal(t, "/etc/privateactionrunner", writer.BaseDir)
}
