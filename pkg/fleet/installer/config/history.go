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
	"time"

	"github.com/pmezard/go-difflib/difflib"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	historyDirName    = ".config_history"
	snapshotDirName   = ".snapshot"
	maxHistoryEntries = 100
)

// history records unified diffs of config changes in a dedicated directory.
type history struct {
	dir string
}

// newHistory initializes the history directory and returns a history recorder.
func newHistory(configDir string) (*history, error) {
	dir := filepath.Join(configDir, historyDirName)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("could not create history directory: %w", err)
	}
	return &history{dir: dir}, nil
}

// record computes unified diffs between stablePath and experimentPath for all YAML
// files and writes a single combined diff entry to the history directory.
// It then updates the snapshot to the experiment state so that a subsequent
// startup check does not re-record the same RC-pushed changes.
func (h *history) record(deploymentID, stablePath, experimentPath string) error {
	before, err := collectYAMLFiles(stablePath)
	if err != nil {
		return fmt.Errorf("could not collect stable yaml files: %w", err)
	}
	after, err := collectYAMLFiles(experimentPath)
	if err != nil {
		return fmt.Errorf("could not collect experiment yaml files: %w", err)
	}
	if _, err := h.writeDiffEntry(deploymentID, before, after); err != nil {
		return err
	}
	return h.updateSnapshot(experimentPath)
}

// recordFromSnapshot compares the current stablePath against the persisted
// snapshot. If the files differ (i.e. local edits were made since the last
// recorded entry), it writes a diff entry tagged "local" and refreshes the
// snapshot. On the very first call (no snapshot yet) it silently initialises
// the snapshot without writing an entry.
func (h *history) recordFromSnapshot(stablePath string) error {
	snapshotDir := filepath.Join(h.dir, snapshotDirName)

	snapshot, err := collectYAMLFiles(snapshotDir)
	if err != nil {
		return fmt.Errorf("could not read snapshot: %w", err)
	}
	current, err := collectYAMLFiles(stablePath)
	if err != nil {
		return fmt.Errorf("could not collect stable yaml files: %w", err)
	}

	// First run: no snapshot yet — initialise silently.
	if len(snapshot) == 0 {
		return h.updateSnapshot(stablePath)
	}

	wrote, err := h.writeDiffEntry("local", snapshot, current)
	if err != nil {
		return err
	}
	if wrote {
		return h.updateSnapshot(stablePath)
	}
	return nil
}

// writeDiffEntry builds a combined unified diff from before/after YAML maps and
// writes it to the history directory. Returns true if an entry was written.
func (h *history) writeDiffEntry(deploymentID string, before, after map[string][]byte) (bool, error) {
	seen := make(map[string]struct{}, len(before)+len(after))
	for p := range before {
		seen[p] = struct{}{}
	}
	for p := range after {
		seen[p] = struct{}{}
	}
	allPaths := make([]string, 0, len(seen))
	for p := range seen {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	now := time.Now().UTC()
	var sb strings.Builder
	fmt.Fprintf(&sb, "# deployment_id: %s\n", deploymentID)
	fmt.Fprintf(&sb, "# timestamp: %s\n", now.Format(time.RFC3339))

	hasDiff := false
	for _, relPath := range allPaths {
		ud := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(before[relPath])),
			B:        difflib.SplitLines(string(after[relPath])),
			FromFile: relPath,
			ToFile:   relPath,
			Context:  3,
		}
		diffText, err := difflib.GetUnifiedDiffString(ud)
		if err != nil {
			return false, fmt.Errorf("could not compute diff for %s: %w", relPath, err)
		}
		if diffText == "" {
			continue
		}
		hasDiff = true
		sb.WriteString("\n")
		sb.WriteString(diffText)
	}

	if !hasDiff {
		return false, nil
	}

	fileName := fmt.Sprintf("%018d.diff", now.UnixNano())
	if err := os.WriteFile(filepath.Join(h.dir, fileName), []byte(sb.String()), 0640); err != nil {
		return false, fmt.Errorf("could not write history entry: %w", err)
	}
	return true, h.prune()
}

// updateSnapshot replaces the snapshot with a fresh copy of all YAML files in dir.
func (h *history) updateSnapshot(dir string) error {
	snapshotDir := filepath.Join(h.dir, snapshotDirName)
	if err := os.RemoveAll(snapshotDir); err != nil {
		return fmt.Errorf("could not clear snapshot: %w", err)
	}
	files, err := collectYAMLFiles(dir)
	if err != nil {
		return fmt.Errorf("could not collect yaml files for snapshot: %w", err)
	}
	for relPath, content := range files {
		destPath := filepath.Join(snapshotDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0750); err != nil {
			return fmt.Errorf("could not create snapshot subdirectory: %w", err)
		}
		if err := os.WriteFile(destPath, content, 0640); err != nil {
			return fmt.Errorf("could not write snapshot file %s: %w", relPath, err)
		}
	}
	return nil
}

// prune removes the oldest entries keeping at most maxHistoryEntries.
func (h *history) prune() error {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return fmt.Errorf("could not read history directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".diff") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if len(files) <= maxHistoryEntries {
		return nil
	}

	for _, name := range files[:len(files)-maxHistoryEntries] {
		if err := os.Remove(filepath.Join(h.dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove old history entry %s: %w", name, err)
		}
	}
	return nil
}

// collectYAMLFiles returns the byte content of every .yaml file under dir,
// keyed by path relative to dir. The history directory itself is skipped.
// A non-existent dir is treated as empty (returns empty map, no error).
func collectYAMLFiles(dir string) (map[string][]byte, error) {
	result := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == historyDirName {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[relPath] = content
		return nil
	})
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}

// isHistoryEnabled reads datadog.yaml from stablePath and reports whether
// config_history.enabled is set to true.
func isHistoryEnabled(stablePath string) bool {
	data, err := os.ReadFile(filepath.Join(stablePath, "datadog.yaml"))
	if err != nil {
		return false
	}
	var cfg struct {
		ConfigHistory struct {
			Enabled bool `yaml:"enabled"`
		} `yaml:"config_history"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Warnf("fleet-installer: could not parse datadog.yaml for config_history flag: %v", err)
		return false
	}
	return cfg.ConfigHistory.Enabled
}
