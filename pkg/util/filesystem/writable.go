// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package filesystem

import (
	"fmt"
	"os"
)

// CheckWritable verifies if a directory is writable by the current process.
// It attempts to create a temporary file in the directory.
func CheckWritable(dir string) error {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory check failed: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	// Try to create a temp file
	tempFile, err := os.CreateTemp(dir, ".agent-write-test-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}

	// Clean up
	name := tempFile.Name()
	_ = tempFile.Close()
	_ = os.Remove(name)

	return nil
}
