// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package processmanager implements fleet installer helpers for dd-procmgr's processes.d catalog
// on macOS.
package processmanager

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
)

// WriteDarwinProcmgrCatalog writes the embedded macOS processes.d entries under installRoot.
// Each entry is written the same way as on Linux, through service.WriteProcess.
func WriteDarwinProcmgrCatalog(installRoot string) error {
	names, err := embedded.DarwinProcmgrProcesses()
	if err != nil {
		return fmt.Errorf("failed to list the embedded processes.d entries: %w", err)
	}
	var errs []error
	for _, name := range names {
		content, err := embedded.GetDarwinProcmgrProcess(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := service.WriteProcess(installRoot, name, content); err != nil {
			errs = append(errs, err)
		}
	}
	// WriteProcess chmods the file, not the directory. Chmod here undoes the umask so processes.d
	// is 0755 on a root install.
	dir := filepath.Join(installRoot, service.ProcessesDirName)
	if err := os.Chmod(dir, 0o755); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, fmt.Errorf("failed to set the mode of %s: %w", dir, err))
	}
	return errors.Join(errs...)
}

// RemoveDarwinProcmgrCatalog removes the entries WriteDarwinProcmgrCatalog writes and removes
// processes.d if that leaves it empty. A path that is not a real directory is left alone, so a
// symlink is not followed out of the install root. Missing files are not errors.
func RemoveDarwinProcmgrCatalog(installRoot string) error {
	dir := filepath.Join(installRoot, service.ProcessesDirName)
	fi, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return nil
	}

	names, err := embedded.DarwinProcmgrProcesses()
	if err != nil {
		return fmt.Errorf("failed to list the embedded processes.d entries: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	var errs []error
	for _, name := range names {
		if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", filepath.Join(dir, name), err))
		}
	}
	empty, err := isEmptyDir(root)
	if err != nil {
		errs = append(errs, err)
	} else if empty {
		if err := os.Remove(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("failed to remove %s: %w", dir, err))
		}
	}
	return errors.Join(errs...)
}

func isEmptyDir(root *os.Root) (bool, error) {
	dir, err := root.Open(".")
	if err != nil {
		return false, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(1)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return len(entries) == 0, nil
}
