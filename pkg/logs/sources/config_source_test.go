// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func CreateTestFile(tempDir string) *os.File {
	// Ensure the directory exists
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return nil
	}

	// Specify the exact file name
	filePath := tempDir + "/config.yaml"

	// Create the file with the specified name
	tempFile, err := os.Create(filePath)
	if err != nil {
		return nil
	}

	// Write the content to the file
	configContent := `logs:
  - type: file
    path: "/tmp/test.log"
    service: "custom_logs"
    source: "custom"`

	_, err = tempFile.Write([]byte(configContent))
	if err != nil {
		tempFile.Close() // Close file before returning
		return nil
	}

	// Close the file after writing
	tempFile.Close()

	// Reopen the file for returning if needed
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	return file
}

func TestSubscribeForTypeAndAddFileSource(t *testing.T) {
	tempDir := "tmp/"
	tempFile := CreateTestFile(tempDir)
	defer os.RemoveAll(tempDir)
	defer os.Remove(tempFile.Name())

	wd, err := os.Getwd()
	assert.NoError(t, err)
	absolutePath := wd + "/" + tempFile.Name()
	data, err := os.ReadFile(absolutePath)
	assert.NoError(t, err)
	logsConfig, err := logsConfig.ParseYAML(data)
	assert.NoError(t, err)
	configSource := NewConfigSources()
	for _, cfg := range logsConfig {
		source := NewLogSource("test-config-name", cfg)
		configSource.AddSource(source)
	}

	addedChan, _ := configSource.SubscribeForType("file")
	added := <-addedChan
	assert.NotNil(t, added)
	assert.Equal(t, "file", added.Config.Type)
	assert.Equal(t, "/tmp/test.log", added.Config.Path)
}
