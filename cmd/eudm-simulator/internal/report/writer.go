// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Writer replaces snapshots at an exclusively reserved report path. The engine
// supplies owned snapshots and serializes calls to Write.
type Writer struct {
	path string
}

// NewWriter reserves a private report path without retaining an open handle.
// Existing reports, including empty files and symlinks, are never overwritten.
func NewWriter(path string) (*Writer, error) {
	reservation, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	if err := reservation.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &Writer{path: path}, nil
}

// Write publishes a complete snapshot by renaming a synced, closed temporary
// file in the same directory. Failed writes leave the previous snapshot intact.
// Concurrent Windows readers must allow delete sharing, as filesystem.OpenShared
// does; a reader without that sharing can prevent replacement.
func (w *Writer) Write(report *Report) error {
	if report == nil {
		return errors.New("local run report must not be nil")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode local run report: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(w.path), ".eudm-report-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, w.path)
}
