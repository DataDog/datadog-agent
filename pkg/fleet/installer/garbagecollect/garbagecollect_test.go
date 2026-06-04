// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package garbagecollect

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupTmpDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	entries := []struct {
		name       string
		path       string
		isDir      bool
		age        time.Duration
		wantExists bool
	}{
		{name: "old file", path: filepath.Join(tmpDir, "old-file"), age: -25 * time.Hour},
		{name: "old directory", path: filepath.Join(tmpDir, "old-dir"), isDir: true, age: -25 * time.Hour},
		{name: "recent file", path: filepath.Join(tmpDir, "recent-file"), age: -1 * time.Hour, wantExists: true},
	}

	for _, entry := range entries {
		if entry.isDir {
			require.NoError(t, os.MkdirAll(filepath.Join(entry.path, "subdir"), 0755))
		} else {
			require.NoError(t, os.WriteFile(entry.path, []byte(entry.name), 0644))
		}
		entryTime := time.Now().Add(entry.age)
		require.NoError(t, os.Chtimes(entry.path, entryTime, entryTime))
	}

	require.NoError(t, cleanupTmpDirectory(tmpDir))

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			if entry.wantExists {
				assert.FileExists(t, entry.path)
			} else if entry.isDir {
				assert.NoDirExists(t, entry.path)
			} else {
				assert.NoFileExists(t, entry.path)
			}
		})
	}
}

func TestCleanupTmpDirectoryIgnoresMissingDirectory(t *testing.T) {
	require.NoError(t, cleanupTmpDirectory(filepath.Join(t.TempDir(), "missing")))
}
