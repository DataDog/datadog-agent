// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package repository

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// copyFileWithPermissions copies a file from src to dst with the same permissions.
func copyFileWithPermissions(src, dst string, info os.FileInfo) error {
	// open source file
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer source.Close()

	var stat *syscall.Stat_t
	var ok bool
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return fmt.Errorf("could not get file stat")
	}

	// Ensure the destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// create dst file with same permissions
	var dstFile *os.File
	dstFile, err = os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// copy content
	if _, err = io.Copy(dstFile, source); err != nil {
		return err
	}

	// set ownership
	if err = os.Chown(dst, int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}

	return nil
}
