// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procmgrdecision

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

// owner has no fields on Windows. The trust check is the owner of the object,
// which paths.IsDirSecure reads.
type owner struct{}

// DefaultStore is the production store: C:\ProgramData\Datadog\Installer\process_manager
// by default. paths.DatadogInstallerData is set at startup, so the path is computed
// here and not in a package variable.
func DefaultStore() *Store {
	return &Store{path: filepath.Join(paths.DatadogInstallerData, recordName), owner: owner{}}
}

// checkDir runs the Windows directory checks of section 5.2. exists is false when
// the directory does not exist, which is not an error for a reader.
func (s *Store) checkDir() (exists bool, err error) {
	dir := s.dir()
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", dir, err)
	}
	// Compare the type, not IsDir: a directory reparse point also carries ModeIrregular.
	if fi.Mode().Type() != fs.ModeDir {
		return true, fmt.Errorf("%w: %s is not a plain directory", ErrUntrusted, dir)
	}
	if err := paths.IsDirSecure(dir); err != nil {
		return true, fmt.Errorf("%w: %v", ErrUntrusted, err)
	}
	return true, nil
}

// openRecord runs the Windows checks of section 5.2 and opens the record. found is
// false when the directory or the file does not exist.
func (s *Store) openRecord() (*os.File, bool, error) {
	exists, err := s.checkDir()
	if err != nil || !exists {
		return nil, false, err
	}
	// Lstat, not Stat: a symlink or a reparse point at the record is not a regular file.
	fi, err := os.Lstat(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("checking %s: %w", s.path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%w: %s is not a regular file", ErrUntrusted, s.path)
	}
	// IsDirSecure reads the owner of the object at the path, so it applies to files too.
	if err := paths.IsDirSecure(s.path); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrUntrusted, err)
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, false, fmt.Errorf("opening %s: %w", s.path, err)
	}
	return f, true, nil
}

// prepareDir creates or repairs the record directory through paths.EnsureInstallerDataDir
// and verifies it. Windows has no directory fsync, so there is no handle to return.
func (s *Store) prepareDir() (*os.File, error) {
	if err := paths.EnsureInstallerDataDir(); err != nil {
		return nil, fmt.Errorf("preparing %s: %w", s.dir(), err)
	}
	exists, err := s.checkDir()
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%s does not exist after it was prepared", s.dir())
	}
	return nil, nil
}

// chmodRecord is a no-op on Windows: the mode is not a permission bit there, the
// ACL is inherited from the installer data directory.
func chmodRecord(_ *os.File) error {
	return nil
}

// syncDir is a no-op on Windows, which has no directory fsync.
func syncDir(_ *os.File) error {
	return nil
}
