// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfigDirIsTheMacOSAgentsOwn(t *testing.T) {
	assert.Equal(t, "/opt/datadog-agent/etc", agentConfigDir)
}

func TestTelemetryConfigReadsTheMacOSAgentConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: abcdef\nsite: datadoghq.eu\n"), 0o600))
	original := agentConfigDir
	agentConfigDir = dir
	t.Cleanup(func() { agentConfigDir = original })

	config, err := telemetryConfig()

	require.NoError(t, err)
	assert.Equal(t, "abcdef", config.APIKey)
	assert.Equal(t, "datadoghq.eu", config.Site)
}
