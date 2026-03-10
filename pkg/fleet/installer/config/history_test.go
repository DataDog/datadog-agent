// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHistory(t *testing.T) {
	dir := t.TempDir()
	h, err := newHistory(dir)
	require.NoError(t, err)
	assert.DirExists(t, filepath.Join(dir, historyDirName))
	assert.NotNil(t, h)
}

func TestHistoryRecord_WritesDiffFile(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: old\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "datadog.yaml"), []byte("api_key: new\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	err = h.record("deploy-1", stableDir, expDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	var diffFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffFiles = append(diffFiles, e.Name())
		}
	}
	require.Len(t, diffFiles, 1)

	content, err := os.ReadFile(filepath.Join(stableDir, historyDirName, diffFiles[0]))
	require.NoError(t, err)

	body := string(content)
	assert.Contains(t, body, "# deployment_id: deploy-1")
	assert.Contains(t, body, "# timestamp:")
	assert.Contains(t, body, "datadog.yaml")
	assert.Contains(t, body, "-api_key: old")
	assert.Contains(t, body, "+api_key: new")
}

func TestHistoryRecord_NoFileWrittenWhenNoDiff(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	content := []byte("api_key: same\n")
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), content, 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "datadog.yaml"), content, 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	err = h.record("deploy-1", stableDir, expDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".diff"), "no diff expected when content is identical")
	}
}

func TestHistoryRecord_MultipleFilesInOneDiff(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stableDir, "conf.d"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(expDir, "conf.d"), 0755))

	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("k: v1\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "datadog.yaml"), []byte("k: v2\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "conf.d", "check.yaml"), []byte("enabled: false\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "conf.d", "check.yaml"), []byte("enabled: true\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	err = h.record("deploy-multi", stableDir, expDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	var diffFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffFiles = append(diffFiles, e.Name())
		}
	}
	// All file diffs in a single entry.
	require.Len(t, diffFiles, 1)

	body, err := os.ReadFile(filepath.Join(stableDir, historyDirName, diffFiles[0]))
	require.NoError(t, err)
	assert.Contains(t, string(body), "datadog.yaml")
	assert.Contains(t, string(body), filepath.Join("conf.d", "check.yaml"))
}

func TestHistoryRecord_NewFileShowsFullContentAsAddition(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	// File only exists in experiment (newly created by an operation).
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "otel-config.yaml"), []byte("receivers: []\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	err = h.record("deploy-new", stableDir, expDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	var diffFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffFiles = append(diffFiles, e.Name())
		}
	}
	require.Len(t, diffFiles, 1)

	body, err := os.ReadFile(filepath.Join(stableDir, historyDirName, diffFiles[0]))
	require.NoError(t, err)
	assert.Contains(t, string(body), "+receivers: []")
}

func TestHistoryRecord_DeletedFileShowsFullContentAsRemoval(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	// File only exists in stable (deleted by an operation).
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "otel-config.yaml"), []byte("receivers: []\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	err = h.record("deploy-del", stableDir, expDir)
	require.NoError(t, err)

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	body, err := os.ReadFile(filepath.Join(stableDir, historyDirName, entries[0].Name()))
	require.NoError(t, err)
	assert.Contains(t, string(body), "-receivers: []")
}

func TestHistoryPrune_KeepsLast100(t *testing.T) {
	dir := t.TempDir()
	h, err := newHistory(dir)
	require.NoError(t, err)

	// Pre-populate 105 diff files.
	for i := range 105 {
		name := fmt.Sprintf("%018d.diff", i)
		require.NoError(t, os.WriteFile(filepath.Join(h.dir, name), []byte("# placeholder"), 0640))
	}

	require.NoError(t, h.prune())

	remaining, err := os.ReadDir(h.dir)
	require.NoError(t, err)
	assert.Len(t, remaining, 100)

	// Oldest files (0–4) must be gone; newest (5–104) must survive.
	names := make([]string, 0, len(remaining))
	for _, e := range remaining {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	assert.Equal(t, fmt.Sprintf("%018d.diff", 5), names[0])
	assert.Equal(t, fmt.Sprintf("%018d.diff", 104), names[99])
}

func TestHistoryRecord_HistoryDirSkippedInCollect(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("k: v1\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "datadog.yaml"), []byte("k: v2\n"), 0640))

	// Plant a .yaml file inside the history dir to ensure it is not collected.
	histDir := filepath.Join(stableDir, historyDirName)
	require.NoError(t, os.MkdirAll(histDir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(histDir, "stray.yaml"), []byte("should: be ignored\n"), 0640))

	files, err := collectYAMLFiles(stableDir)
	require.NoError(t, err)

	for p := range files {
		assert.False(t, strings.Contains(p, historyDirName), "history dir must not appear in collected files")
	}
}

func TestHistoryRecordFromSnapshot_FirstRun_InitialisesSnapshotSilently(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: abc\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	// No snapshot exists yet — should silently create one without writing a diff.
	require.NoError(t, h.recordFromSnapshot(stableDir))

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	diffFiles := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffFiles++
		}
	}
	assert.Equal(t, 0, diffFiles, "first run must not write a diff entry")

	// Snapshot should now contain the stable file.
	snapshotContent, err := os.ReadFile(filepath.Join(stableDir, historyDirName, snapshotDirName, "datadog.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "api_key: abc\n", string(snapshotContent))
}

func TestHistoryRecordFromSnapshot_LocalEdit_RecordsDiff(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: original\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)

	// Initialise snapshot.
	require.NoError(t, h.recordFromSnapshot(stableDir))

	// Simulate a local edit to the stable file.
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: edited\n"), 0640))

	// Second startup — should detect the change.
	require.NoError(t, h.recordFromSnapshot(stableDir))

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	var diffFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffFiles = append(diffFiles, e.Name())
		}
	}
	require.Len(t, diffFiles, 1)

	body, err := os.ReadFile(filepath.Join(stableDir, historyDirName, diffFiles[0]))
	require.NoError(t, err)
	assert.Contains(t, string(body), "# deployment_id: local")
	assert.Contains(t, string(body), "-api_key: original")
	assert.Contains(t, string(body), "+api_key: edited")
}

func TestHistoryRecordFromSnapshot_NoChange_NoDiff(t *testing.T) {
	stableDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: same\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)
	require.NoError(t, h.recordFromSnapshot(stableDir))

	// Second startup — nothing changed.
	require.NoError(t, h.recordFromSnapshot(stableDir))

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasSuffix(e.Name(), ".diff"), "no diff expected when nothing changed")
	}
}

func TestHistoryRecord_UpdatesSnapshot(t *testing.T) {
	stableDir := t.TempDir()
	expDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: old\n"), 0640))
	require.NoError(t, os.WriteFile(filepath.Join(expDir, "datadog.yaml"), []byte("api_key: new\n"), 0640))

	h, err := newHistory(stableDir)
	require.NoError(t, err)
	require.NoError(t, h.record("deploy-1", stableDir, expDir))

	// Snapshot should reflect the experiment state.
	snapshotContent, err := os.ReadFile(filepath.Join(stableDir, historyDirName, snapshotDirName, "datadog.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "api_key: new\n", string(snapshotContent))

	// After promote (stable = experiment), startup should not record a duplicate.
	require.NoError(t, os.WriteFile(filepath.Join(stableDir, "datadog.yaml"), []byte("api_key: new\n"), 0640))
	require.NoError(t, h.recordFromSnapshot(stableDir))

	entries, err := os.ReadDir(filepath.Join(stableDir, historyDirName))
	require.NoError(t, err)
	diffCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".diff") {
			diffCount++
		}
	}
	assert.Equal(t, 1, diffCount, "promote then restart must not produce a duplicate entry")
}

func TestIsHistoryEnabled(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("config_history:\n  enabled: true\n"), 0640))
		assert.True(t, isHistoryEnabled(dir))
	})

	t.Run("disabled", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("config_history:\n  enabled: false\n"), 0640))
		assert.False(t, isHistoryEnabled(dir))
	})

	t.Run("missing_key", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte("api_key: abc\n"), 0640))
		assert.False(t, isHistoryEnabled(dir))
	})

	t.Run("missing_file", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, isHistoryEnabled(dir))
	})
}
