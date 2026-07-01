// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoStackConfig is returned by LoadStackConfig when no persisted config
// exists for the given stack name.
var ErrNoStackConfig = errors.New("no persisted stack config")

// stateDir returns the directory used to persist per-stack provisioning configs.
// The SCENARIORUN_STATE_DIR environment variable overrides the default.
func stateDir() string {
	if d := os.Getenv("SCENARIORUN_STATE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scenariorun", "stacks")
}

// sanitizeStackName replaces characters that are unsafe in file names (path
// separators, spaces) with underscores so the stack name can be used as a
// plain file base-name.
func sanitizeStackName(stack string) string {
	r := strings.NewReplacer(
		string(filepath.Separator), "_",
		"/", "_",
		"\\", "_",
		" ", "_",
	)
	return r.Replace(stack)
}

func stackConfigPath(stack string) (string, error) {
	dir := stateDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create state dir %q: %w", dir, err)
	}
	return filepath.Join(dir, sanitizeStackName(stack)+".json"), nil
}

// SaveStackConfig persists cfg (the provisioning map[string]string) to disk so
// that RunAction and Destroy can replay the same topology.
func SaveStackConfig(stack string, cfg map[string]string) error {
	path, err := stackConfigPath(stack)
	if err != nil {
		return err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal stack config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write stack config %q: %w", path, err)
	}
	return nil
}

// LoadStackConfig reads back the provisioning config that was persisted by
// SaveStackConfig. It returns ErrNoStackConfig (wrapped) if the file is absent.
func LoadStackConfig(stack string) (map[string]string, error) {
	dir := stateDir()
	path := filepath.Join(dir, sanitizeStackName(stack)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w for stack %q", ErrNoStackConfig, stack)
		}
		return nil, fmt.Errorf("read stack config %q: %w", path, err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal stack config %q: %w", path, err)
	}
	return cfg, nil
}

// DeleteStackConfig removes the persisted config for stack. It is a no-op if
// the file does not exist.
func DeleteStackConfig(stack string) error {
	dir := stateDir()
	path := filepath.Join(dir, sanitizeStackName(stack)+".json")
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete stack config %q: %w", path, err)
	}
	return nil
}
