// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"io"
	"os"
	"path/filepath"
)

// copyFileWithPermissions copies a file from src to dst with the same permissions.
func copyFileWithPermissions(src, dst string, _ os.FileInfo) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
