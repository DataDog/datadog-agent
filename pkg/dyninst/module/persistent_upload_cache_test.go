// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestNewPersistentUploadCache_CleanupOnInit(t *testing.T) {
	cacheDir := t.TempDir()

	// Define which PIDs should be considered "alive" for this test.
	alivePIDs := map[int]bool{
		1000: true,
		2000: true,
	}

	// Create various test files.
	testFiles := []struct {
		name        string
		content     interface{}
		shouldExist bool
		description string
	}{
		{
			name: "1000.json",
			content: uploadEntry{
				Type:           entryTypeCompleted,
				PID:            1000,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				AgentVersion:   version.AgentVersion,
				Timestamp:      time.Now(),
			},
			shouldExist: true,
			description: "valid entry for alive process with current agent version",
		},
		{
			name: "2000.json",
			content: uploadEntry{
				Type:           entryTypeAttempt,
				PID:            2000,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				AgentVersion:   version.AgentVersion,
				Timestamp:      time.Now(),
				ErrorNumber:    1,
				Error:          "some error",
			},
			shouldExist: true,
			description: "valid attempt entry for alive process - should be updated with restart error",
		},
		{
			name: "3000.json",
			content: uploadEntry{
				Type:           entryTypeCompleted,
				PID:            3000,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				AgentVersion:   version.AgentVersion,
				Timestamp:      time.Now(),
			},
			shouldExist: false,
			description: "entry for dead process",
		},
		{
			name: "4000.json",
			content: uploadEntry{
				Type:           entryTypeCompleted,
				PID:            4000,
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				AgentVersion:   "3.2.1", // Different version.
				Timestamp:      time.Now(),
			},
			shouldExist: false,
			description: "entry with old agent version",
		},
		{
			name:        "invalid.json",
			content:     "not valid json content",
			shouldExist: false,
			description: "file with invalid JSON",
		},
		{
			name:        "5000.txt",
			content:     "some text content",
			shouldExist: false,
			description: "file with wrong extension",
		},
		{
			name:        "notanumber.json",
			content:     `{"type": 0}`,
			shouldExist: false,
			description: "file with non-numeric PID in filename",
		},
		{
			name: "6000.json",
			content: map[string]interface{}{
				"type":         999, // Invalid entry type.
				"pid":          6000,
				"agentversion": version.AgentVersion,
			},
			shouldExist: false,
			description: "entry with invalid entry type",
		},
	}

	// Create test files.
	for _, tf := range testFiles {
		filePath := filepath.Join(cacheDir, tf.name)
		var data []byte
		var err error

		switch content := tf.content.(type) {
		case uploadEntry:
			data, err = json.Marshal(content)
			require.NoError(t, err, "failed to marshal %s", tf.description)
		case string:
			data = []byte(content)
		case map[string]interface{}:
			data, err = json.Marshal(content)
			require.NoError(t, err, "failed to marshal %s", tf.description)
		}

		err = os.WriteFile(filePath, data, 0644)
		require.NoError(t, err, "failed to write test file %s", tf.name)
	}

	// Create a directory to test cleanup of unexpected directories.
	unexpectedDir := filepath.Join(cacheDir, "unexpected_directory")
	err := os.Mkdir(unexpectedDir, 0755)
	require.NoError(t, err)

	// Initialize the cache with process existence check.
	cache, err := newPersistentUploadCache(cacheDir, withProcessExistsCheck(func(pid int) bool {
		return alivePIDs[pid]
	}))
	require.NoError(t, err)
	require.Equal(t, cacheDir, cache.dir)

	// Verify that files exist or don't exist as expected.
	for _, tf := range testFiles {
		filePath := filepath.Join(cacheDir, tf.name)
		_, err := os.Stat(filePath)
		if tf.shouldExist {
			require.NoError(t, err, "%s should exist: %s", tf.name, tf.description)
		} else {
			require.True(t, os.IsNotExist(err), "%s should be deleted: %s", tf.name, tf.description)
		}
	}

	// Verify unexpected directory was removed.
	_, err = os.Stat(unexpectedDir)
	require.True(t, os.IsNotExist(err), "unexpected directory should be deleted")

	// Verify that the attempt entry for PID 2000 was updated with restart error.
	entry, err := cache.GetEntry(2000)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, entryTypeAttempt, entry.Type)
	require.Equal(t, "agent restarted during upload", entry.Error)

	// Verify that the completed entry for PID 1000 was not modified.
	entry, err = cache.GetEntry(1000)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, entryTypeCompleted, entry.Type)
}

func TestNewPersistentUploadCache_NonExistentDirectory(t *testing.T) {
	parentDir := t.TempDir()
	cacheDir := filepath.Join(parentDir, "nested", "cache")

	cache, err := newPersistentUploadCache(cacheDir)
	require.NoError(t, err)
	require.Equal(t, cacheDir, cache.dir)

	// Verify directory was created.
	_, err = os.Stat(cacheDir)
	require.NoError(t, err)
}
