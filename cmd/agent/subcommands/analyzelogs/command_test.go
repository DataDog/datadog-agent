// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package analyzelogs

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"analyze-logs", "path/to/log/config.yaml"},
		runAnalyzeLogs,
		func(_ core.BundleParams, cliParams *CliParams) {
			require.Equal(t, "path/to/log/config.yaml", cliParams.LogConfigPath)
			require.Equal(t, defaultCoreConfigPath, cliParams.CoreConfigPath)
		})
}

func TestRunAnalyzeLogs(t *testing.T) {
	tempDir := "tmp/"
	err := os.MkdirAll(tempDir, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir) // Cleanup the temp directory after the test

	// Create a temporary config file
	tempFile, err := os.CreateTemp(tempDir, "config.yaml")
	assert.NoError(t, err)
	defer os.Remove(tempFile.Name()) // Cleanup the temp file after the test

	// Write config content to the temp file
	configContent := `logs:
  - type: file
	path: "/tmp/test.log"
	service: "custom_logs"
	source: "custom"`

	_, err = tempFile.Write([]byte(configContent))
	assert.NoError(t, err)
	tempFile.Close()

	// Create a mock config
	config := config.NewMock(t)

	// Set CLI params
	cliParams := &CliParams{
		CoreConfigPath: "bin/agent/dist/datadog.yaml",
		LogConfigPath:  tempFile.Name(),
		ConfigSource:   sources.GetInstance(),
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stdout = w

	// Run the function
	go func() {
		err := runAnalyzeLogs(cliParams, config)
		assert.NoError(t, err)
		w.Close() // Close the write end of the pipe when done
	}()

	// Read and verify the output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	assert.NoError(t, err)
	os.Stdout = oldStdout // Restore original stdout

	// Assert output matches expected
	assert.Equal(t, configContent, buf.String())
}
