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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestSingletonInstance(t *testing.T) {
	instance1 := GetInstance()
	instance2 := GetInstance()
	assert.Equal(t, instance1, instance2, "GetInstance should return the same instance")
}

func CreateTestFile(tempDir string) *os.File {
	err := os.MkdirAll(tempDir, 0755)
	if err != nil {
		return nil
	}

	// Create a temporary file
	tempFile, err := os.CreateTemp(tempDir, "config.yaml")
	if err != nil {
		return nil
	}
	configContent := `logs:
  - type: file
    path: "/tmp/test.log"
    service: "custom_logs"
    source: "custom"`

	_, err = tempFile.Write([]byte(configContent))
	if err != nil {
		return nil
	}
	tempFile.Close()
	return tempFile
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

func TestSubscribeAllConfig(t *testing.T) {
	// Create a temporary directory and file for the config
	tempDir := "tmp/"
	tempFile := CreateTestFile(tempDir)
	defer os.RemoveAll(tempDir)
	defer os.Remove(tempFile.Name())
	fmt.Println("wack1")
	log.Debug("WACKTEST1")
	// Add the file source
	configSource := GetInstance()
	fmt.Println("wack2")
	log.Debug("WACKTEST2")
	err := configSource.AddFileSource(tempFile.Name())
	fmt.Println("wack3")
	log.Debug("WACKTEST3")
	assert.NoError(t, err)
	fmt.Println("wack4")
	log.Debug("WACKTEST4")
	// Subscribe for all sources
	addedChan, _ := configSource.SubscribeAll()
	fmt.Println("wack5")
	log.Debug("WACKTEST5")
	for {
		select {
		case added := <-addedChan:
			fmt.Println("CHANNEL IM READING FROM IS ", addedChan)
			// Check the type and path of the source added
			assert.Equal(t, "file", added.Config.Type)
			return
		case <-time.After(10 * time.Second):
			fmt.Println("ERROR TIME OUT")
			log.Debug("ERROR TIME OUT 2")
			t.Fatal("Timeout waiting for source addition")
			return
		}
	}
}

func TestGetAddedForTypeConfig(t *testing.T) {
	// Create a temporary directory and file for the config
	tempDir := "tmp/"
	tempFile := CreateTestFile(tempDir)
	defer os.RemoveAll(tempDir)
	defer os.Remove(tempFile.Name())

	// Add the file source
	configSource := GetInstance()
	err := configSource.AddFileSource(tempFile.Name())
	assert.NoError(t, err)

	// Get the sources added for a specific type
	sources := configSource.GetAddedForType("file")

	select {
	case added := <-sources:
		// Check the type and path of the source added
		assert.Equal(t, "file", added.Config.Type)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for source addition")
	}
}
