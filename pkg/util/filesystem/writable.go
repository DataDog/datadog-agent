// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// IsReadOnly is used to verify if a path is mounted as a read-only filesystem by
// creating a temp file in the directory then deleting it after the check.
//
// Using os.Stat() is not a reliable way to check if a path is writable since it
// ignores how the backing filesystem is mounted (e.g. read-only). Instead, attempting
// a write operation is more reliable.
func IsReadOnly(dir string) (bool, error) {
	if !FileExists(dir) {
		// A missing directory does not mean the path is read-only.
		err := os.Mkdir(dir, 0755)
		// If we can't create the directory, it's not writable.
		if os.IsPermission(err) || errors.Is(err, syscall.EROFS) {
			return false, nil
		}

		if err != nil {
			return false, fmt.Errorf("failed to create directory: %w", err)
		}
	}

	tempFile, err := os.CreateTemp(dir, ".agent-write-test-*")
	if err != nil {
		if os.IsPermission(err) || errors.Is(err, syscall.EROFS) {
			return false, nil
		}
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}

	defer func() {
		name := tempFile.Name()
		_ = tempFile.Close()
		_ = os.Remove(name)
	}()
	return true, nil
}
