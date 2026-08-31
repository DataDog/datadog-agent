// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package storeimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// issuesPersistence abstracts loading and saving persisted issue state.
// Implementations load from disk, load from the Agent Health API, or disable
// persistence when neither source is appropriate.
type issuesPersistence interface {
	// load reads persisted state from storage.
	// Returns (nil, nil) when no prior state exists.
	load(context.Context) (*PersistedState, error)
	// save writes the given state to storage.
	save(state *PersistedState) error
}

// diskPersistence persists issue state to a JSON file using an atomic write
// (temp file + rename) to avoid corruption on crash.
type diskPersistence struct {
	path   string
	logger log.Component
}

func newDiskPersistence(path string, logger log.Component) *diskPersistence {
	return &diskPersistence{path: path, logger: logger}
}

func (d *diskPersistence) load(_ context.Context) (*PersistedState, error) {
	data, err := os.ReadFile(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			d.logger.Info("No persisted issues found, starting fresh")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read persisted issues: %w", err)
	}

	var state PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal persisted issues: %w", err)
	}
	return &state, nil
}

func (d *diskPersistence) save(state *PersistedState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal issues: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(d.path), 0755); err != nil {
		return fmt.Errorf("failed to create persistence directory: %w", err)
	}

	tmpPath := d.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, d.path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup; the rename error is the real failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
