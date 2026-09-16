// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"fmt"
	"os"
	"path/filepath"
)

// The Agent configuration directory and package trees are owned by the unprivileged dd-agent
// user, while the install and upgrade hooks run as root. The helpers in this file scope every
// operation to the directory holding the file, so a symlink planted by dd-agent cannot redirect
// a root-run read or write onto an arbitrary file: os.Root rejects absolute symlinks and any
// symlink resolving outside its root. The directory itself is not swappable, it lives under a
// root-owned parent.

// The os.Root methods report only the name they were given, so "openat <name>: ..." would
// reach the install logs with neither the operation nor the directory. Each helper below
// therefore names the operation and the full path. Wrapping uses %w throughout, so callers must
// test for a missing file with errors.Is(err, os.ErrNotExist): os.IsNotExist only unwraps
// *PathError, *LinkError and *SyscallError, and silently reports false for a %w chain.

// readFileInDir reads path without resolving a symlink that leaves path's directory.
func readFileInDir(path string) ([]byte, error) {
	root, err := openDirOf(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := root.ReadFile(filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}
	return data, nil
}

// writeFileInDir writes path without resolving a symlink that leaves path's directory.
func writeFileInDir(path string, data []byte, perm os.FileMode) error {
	root, err := openDirOf(path)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.WriteFile(filepath.Base(path), data, perm); err != nil {
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return nil
}

// lstatInDir stats path without resolving a symlink that leaves path's directory, and without
// resolving path's own final component.
func lstatInDir(path string) (os.FileInfo, error) {
	root, err := openDirOf(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("could not stat %s: %w", path, err)
	}
	return info, nil
}

// openDirOf anchors an os.Root at the directory holding path. The error is returned as is:
// os.OpenRoot names the operation and the full directory path itself.
func openDirOf(path string) (*os.Root, error) {
	return os.OpenRoot(filepath.Dir(path))
}
