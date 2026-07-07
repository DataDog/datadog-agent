// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package statedir contains helpers for creating directories and writing files
// used to persist dynamic instrumentation state. Dynamic instrumentation runs
// as root, so these helpers guard against an unprivileged local user planting a
// directory or symlink that would cause a root-owned write to land on an
// arbitrary file.
package statedir

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// EnsureSecure makes sure dir exists and is safe for root to write into. If the
// directory does not exist yet, it is created (along with any missing parents)
// with 0700 permissions, and no further checks are needed because we just made
// it. If it already exists, it must be a real directory and not a symlink; a
// symlink planted by an unprivileged user could otherwise redirect root's
// writes elsewhere.
func EnsureSecure(dir string) error {
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use %s: it is a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to use %s: it is not a directory", dir)
	}
	return nil
}

// WriteFile writes data to path with 0600 permissions, refusing to follow a
// symlink at the final path component. Combined with EnsureSecure on the parent
// directory, this prevents an unprivileged user from redirecting a root-owned
// write onto a file of their choosing.
func WriteFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
