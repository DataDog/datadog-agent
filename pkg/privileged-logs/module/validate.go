// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func isAllowed(path, allowedPrefix string) bool {
	// File names ending with .log are allowed regardless of where they are
	// located in the file system.
	if strings.ToLower(filepath.Ext(path)) == ".log" {
		return true
	}

	// Files in the allowed prefix are allowed regardless of the file name.
	if !strings.HasSuffix(allowedPrefix, "/") {
		allowedPrefix = allowedPrefix + "/"
	}
	if strings.HasPrefix(path, allowedPrefix) {
		return true
	}

	// Files which have any ancestor directory named "logs" are allowed
	// regardless of the file name.
	dir := filepath.Dir(path)
	parts := strings.SplitSeq(dir, "/")
	for part := range parts {
		if strings.ToLower(part) == "logs" {
			return true
		}
	}

	return false
}

// isTextFile checks if the given file is a text file by reading the first 128 bytes
// and checking if they are valid UTF-8.  Note that empty files are considered
// text files.
func isTextFile(file *os.File) bool {
	buf := make([]byte, 128)
	// ReadAt ensures that the file offset is not modified.
	_, err := file.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return false
	}
	return utf8.Valid(buf)
}

func validateAndOpenWithPrefix(path, allowedPrefix string, toctou func()) (*os.File, error) {
	if path == "" {
		return nil, errors.New("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("relative path not allowed: %s", path)
	}

	// Resolve symbolic links for the path and file name checks.
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	// Callback for tests to change the filesystem after we called EvalSymlinks,
	// in order to simulate a TOCTOU attack.
	if toctou != nil {
		toctou()
	}

	var file *os.File

	if !isAllowed(resolvedPath, allowedPrefix) {
		return nil, fmt.Errorf("non-log file not allowed: %s", resolvedPath)
	}

	// We use openPathWithoutSymlinks on the resolved path to verify each
	// component with O_NOFOLLOW to ensure that none of the path components
	// were replaced with symlinks after we called EvalSymlinks.
	file, err = openPathWithoutSymlinks(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open path %s: %w", resolvedPath, err)
	}

	fi, err := file.Stat()
	if err != nil {
		file.Close()
		// err already contains the path
		return nil, err
	}

	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("not a regular file: %s", resolvedPath)
	}

	if !isTextFile(file) {
		file.Close()
		return nil, fmt.Errorf("not a text file: %s", resolvedPath)
	}

	return file, nil
}

func validateAndOpen(path string) (*os.File, error) {
	return validateAndOpenWithPrefix(path, "/var/log/", nil)
}
