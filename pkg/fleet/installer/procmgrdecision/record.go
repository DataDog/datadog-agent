// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

const (
	// recordName is the file name of the record inside the installer-owned data directory.
	recordName = "process_manager"
	// recordTempPattern is the pattern of the temporary file created next to the record.
	recordTempPattern = ".process_manager.*"
	// recordMaxSize is the size of the largest valid record, newline included.
	recordMaxSize = 64
	// recordReadLimit is how many bytes a reader reads. One byte more than the largest valid record,
	// so an oversized file is detected without reading it all.
	recordReadLimit = recordMaxSize + 1

	recordEnabled  = "v1 enabled"
	recordDisabled = "v1 disabled"
)

// renameRecord replaces the record file. It is a variable so tests can make the rename fail.
var renameRecord = paths.Rename

// Store is the decision record.
type Store struct {
	path string
	owner
}

// Path returns the record file path.
func (s *Store) Path() string {
	return s.path
}

// dir returns the installer-owned directory that holds the record.
func (s *Store) dir() string {
	return filepath.Dir(s.path)
}

// Read returns the recorded decision. found is false when there is no record.
// A nil *Store has no record.
//
// A record that exists but fails the platform trust check returns an error that
// wraps ErrUntrusted. A trusted record with invalid content wraps ErrInvalid.
// Read never falls back to another source: that is the caller's decision.
func (s *Store) Read() (enabled bool, found bool, err error) {
	if s == nil {
		return false, false, nil
	}
	f, found, err := s.openRecord()
	if err != nil || !found {
		return false, false, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, recordReadLimit))
	if err != nil {
		return false, false, fmt.Errorf("reading %s: %w", s.path, err)
	}
	enabled, err = parseRecord(data)
	if err != nil {
		return false, false, fmt.Errorf("%s: %w", s.path, err)
	}
	return enabled, true, nil
}

// Write records the decision. It creates or repairs the directory first and
// verifies it, then replaces the record atomically: a temporary file in the same
// directory is written, synced and renamed over the record. A reader never sees a
// partial record. Writing the value that is already recorded does not touch the
// file.
//
// Write takes no lock: serializing writers is the caller's job.
func (s *Store) Write(ctx context.Context, enabled bool) error {
	dir, err := s.prepareDir()
	if err != nil {
		return err
	}
	if dir != nil {
		defer dir.Close()
	}

	// A trusted, valid record with the same value needs no write. Any read error is ignored:
	// an explicit value must be able to replace a broken record once the directory itself passed.
	if current, found, err := s.Read(); err == nil && found && current == enabled {
		return nil
	}

	content := recordDisabled + "\n"
	if enabled {
		content = recordEnabled + "\n"
	}
	tmp, err := os.CreateTemp(s.dir(), recordTempPattern)
	if err != nil {
		return fmt.Errorf("creating temporary process manager record in %s: %w", s.dir(), err)
	}
	tmpName := tmp.Name()
	if err := writeRecordTemp(tmp, content); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := renameRecord(ctx, tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replacing process manager record %s: %w", s.path, err)
	}
	if err := syncDir(dir); err != nil {
		// The new content is already in place. A retry is safe and idempotent.
		return fmt.Errorf("syncing %s: %w", s.dir(), err)
	}
	return nil
}

// writeRecordTemp writes the content to the open temporary file, sets its mode,
// syncs it and closes it, whatever the outcome.
func writeRecordTemp(f *os.File, content string) (err error) {
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("closing temporary process manager record: %w", closeErr)
		}
	}()
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("writing temporary process manager record: %w", err)
	}
	if err := chmodRecord(f); err != nil {
		return fmt.Errorf("setting mode of temporary process manager record: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing temporary process manager record: %w", err)
	}
	return nil
}

// parseRecord parses the content of a record. Exactly one line, "v1 enabled" or
// "v1 disabled", with or without one trailing newline. Parsing is case-sensitive.
func parseRecord(data []byte) (bool, error) {
	if len(data) > recordMaxSize {
		return false, fmt.Errorf("%w: larger than %d bytes", ErrInvalid, recordMaxSize)
	}
	switch string(bytes.TrimSuffix(data, []byte("\n"))) {
	case recordEnabled:
		return true, nil
	case recordDisabled:
		return false, nil
	default:
		return false, fmt.Errorf("%w: unrecognized content %q", ErrInvalid, data)
	}
}
