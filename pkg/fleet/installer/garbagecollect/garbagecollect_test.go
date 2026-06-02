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

	oldFile := filepath.Join(tmpDir, "old-file")
	recentFile := filepath.Join(tmpDir, "recent-file")
	oldDir := filepath.Join(tmpDir, "old-dir")
	oldDirFile := filepath.Join(oldDir, "file")

	require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0644))
	require.NoError(t, os.WriteFile(recentFile, []byte("recent"), 0644))
	require.NoError(t, os.MkdirAll(oldDir, 0755))
	require.NoError(t, os.WriteFile(oldDirFile, []byte("old"), 0644))

	oldTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(oldFile, oldTime, oldTime))
	require.NoError(t, os.Chtimes(oldDirFile, oldTime, oldTime))
	require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

	require.NoError(t, cleanupTmpDirectory(tmpDir))

	assert.NoFileExists(t, oldFile)
	assert.NoDirExists(t, oldDir)
	assert.FileExists(t, recentFile)
}

func TestCleanupTmpDirectoryIgnoresMissingDirectory(t *testing.T) {
	require.NoError(t, cleanupTmpDirectory(filepath.Join(t.TempDir(), "missing")))
}
