// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func CreateTestFile(tempDir string) *os.File {
	// Ensure the directory exists
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return nil
	}

	// Specify the exact file name
	filePath := fmt.Sprintf("%s/config.yaml", tempDir)

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

	configSource := NewConfigSources()
	err := configSource.AddFileSource(tempFile.Name())
	addedChan, _ := configSource.SubscribeForType("file")
	added := <-addedChan
	assert.NotNil(t, added)
	assert.Equal(t, "file", added.Config.Type)
	assert.Equal(t, "/tmp/test.log", added.Config.Path)

	assert.NoError(t, err)
}
