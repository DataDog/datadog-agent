// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package analyzelogs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
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

func CreateTestFile(tempDir string, fileName string, fileContent string) *os.File {
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		err = os.MkdirAll(tempDir, 0755)
		if err != nil {
			return nil
		}
	}

	filePath := fmt.Sprintf("%s/%s", tempDir, fileName)

	tempFile, err := os.Create(filePath)
	if err != nil {
		return nil
	}

	_, err = tempFile.Write([]byte(fileContent))
	if err != nil {
		tempFile.Close() // Close file before returning
		return nil
	}

	tempFile.Close()

	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	return file
}

func TestRunAnalyzeLogs(t *testing.T) {
	tempDir := "tmp"
	defer os.RemoveAll(tempDir)

	// Write config content to the temp file
	logConfig := `=== apm check ===
Configuration provider: file
Configuration source: file:/opt/datadog-agent/etc/conf.d/apm.yaml.default
Config for instance ID: apm:1234567890abcdef
{}

=== container_image check ===
Configuration provider: file
Configuration source: file:/opt/datadog-agent/etc/conf.d/container_image.d/conf.yaml.default
Config for instance ID: container_image:abcdef1234567890
{}
~
Auto-discovery IDs:
* _container_image
===
`
	// Create a temporary config file
	tempLogFile := CreateTestFile(tempDir, "wack.log", logConfig)
	assert.NotNil(t, tempLogFile)
	defer os.Remove(tempLogFile.Name()) // Cleanup the temp file after the test

	yamlContent := fmt.Sprintf(`logs:
  - type: file
    path: %s
    log_processing_rules:
      - type: exclude_at_match
        name: exclude_random
        pattern: "datadog-agent"
      
`, tempLogFile.Name())
	tempConfigFile := CreateTestFile(tempDir, "config.yaml", yamlContent)
	assert.NotNil(t, tempConfigFile)

	defer os.Remove(tempConfigFile.Name())
	// Write config content to the temp file

	// Create a mock config
	config := config.NewMock(t)

	// Set CLI params
	cliParams := &CliParams{
		LogConfigPath:  tempConfigFile.Name(),
		CoreConfigPath: tempConfigFile.Name(),
	}

	// Wait for code to finish running before trying to read
	time.Sleep(3 * time.Second)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stdout = w

	err = runAnalyzeLogs(cliParams, config)
	assert.NoError(t, err)

	w.Close() // Close the write end of the pipe when done

	// // Read and verify the output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	assert.NoError(t, err)
	os.Stdout = oldStdout // Restore original stdout

	// Assert output matches expected
	expectedOutput := `=== apm check ===
Configuration provider: file
Config for instance ID: apm:1234567890abcdef
{}
=== container_image check ===
Configuration provider: file
Config for instance ID: container_image:abcdef1234567890
{}
~
Auto-discovery IDs:
* _container_image
===`

	// // Use contains isntead of equals since there is also debug logs sent to stdout
	assert.Contains(t, buf.String(), expectedOutput)
}
