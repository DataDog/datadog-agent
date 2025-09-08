// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package symlink contains the logic to manage symlinks.
package symlink

import (
	"errors"
	"os"
)

// Read reads the target of a link.
func Read(linkPath string) (string, error) {
	return os.Readlink(linkPath)
}

// Exist checks if a link exists.
func Exist(linkPath string) (bool, error) {
	_, err := os.Stat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// Set creates a link.
func Set(linkPath string, targetPath string) error {
	return atomicSymlink(targetPath, linkPath)
}

// Delete removes a link.
func Delete(linkPath string) error {
	return os.Remove(linkPath)
}
