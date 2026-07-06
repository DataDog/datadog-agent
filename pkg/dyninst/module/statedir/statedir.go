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
	"fmt"
	"os"
	"syscall"
)

// EnsureSecure creates dir (and any missing parents) and verifies that the
// resulting directory is safe for root to write into: it must be a real
// directory, not a symlink, and not writable by group or other. If an
// unprivileged user could write to the directory, or planted a symlink in its
// place, this returns an error so the caller can refuse to use it.
func EnsureSecure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to use %s: it is a symlink", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to use %s: it is not a directory", dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("refusing to use %s: it is writable by group or other (mode %o)", dir, info.Mode().Perm())
	}
	return nil
}

// WriteFile writes data to path, refusing to follow a symlink at the final path
// component. Combined with EnsureSecure on the parent directory, this prevents
// an unprivileged user from redirecting a root-owned write onto a file of their
// choosing.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
