// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sources

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSingletonInstance(t *testing.T) {
	instance1 := GetInstance()
	instance2 := GetInstance()
	assert.Equal(t, instance1, instance2, "GetInstance should return the same instance")
}

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

func TestAddFileSource(t *testing.T) {
	tempDir := "tmp/"
	tempFile := CreateTestFile("tmp/")
	defer os.RemoveAll(tempDir)
	defer os.Remove(tempFile.Name())

	// Add the file source
	configSource := GetInstance()
	err := configSource.AddFileSource(tempFile.Name())
	assert.NoError(t, err)

	// Validate source added
	assert.Len(t, configSource.sources, 1)
	assert.Equal(t, "file", configSource.sources[0].Config.Type)

	assert.NoError(t, err)
	assert.Equal(t, "/tmp/test.log", configSource.sources[0].Config.Path)
}

func TestSubscribeForTypeConfig(t *testing.T) {
	configSource := GetInstance()
	tempDir := "tmp/"
	tempFile := CreateTestFile("tmp/")
	defer os.RemoveAll(tempDir)
	defer os.Remove(tempFile.Name())

	// Add the file source
	err := configSource.AddFileSource(tempFile.Name())
	assert.NoError(t, err)

	addedChan, _ := configSource.SubscribeForType("file")

	select {
	case added := <-addedChan:
		assert.Equal(t, "file", added.Config.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for source addition of type 'file'")
	}
}
