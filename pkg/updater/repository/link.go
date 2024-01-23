// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"errors"
	"os"
	"path/filepath"
)

func linkRead(linkPath string) (string, error) {
	return filepath.EvalSymlinks(linkPath)
}

func linkExists(linkPath string) (bool, error) {
	_, err := os.Stat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func linkSet(linkPath string, targetPath string) error {
	return atomicSymlink(targetPath, linkPath)
}

func linkDelete(linkPath string) error {
	return os.Remove(linkPath)
}

// atomicSymlink wraps os.Symlink, replacing an existing symlink with the same name
// atomically (os.Symlink fails when newname already exists, at least on Linux).
//
// vendored from https://github.com/google/renameio/blob/v1.0.1/tempfile.go#L156-L187
func atomicSymlink(oldname, newname string) error {
	// Fast path: if newname does not exist yet, we can skip the whole dance
	// below.
	if err := os.Symlink(oldname, newname); err == nil || !os.IsExist(err) {
		return err
	}

	// We need to use ioutil.TempDir, as we cannot overwrite a ioutil.TempFile,
	// and removing+symlinking creates a TOCTOU race.
	d, err := os.MkdirTemp(filepath.Dir(newname), "."+filepath.Base(newname))
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(d)
		}
	}()

	symlink := filepath.Join(d, "tmp.symlink")
	if err := os.Symlink(oldname, symlink); err != nil {
		return err
	}

	if err := os.Rename(symlink, newname); err != nil {
		return err
	}

	cleanup = false
	return os.RemoveAll(d)
}
