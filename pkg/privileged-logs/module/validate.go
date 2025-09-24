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
	"syscall"
	"unicode/utf8"
)

func isLogFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".log")
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

func validateAndOpenWithPrefix(path, allowedPrefix string) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("relative path not allowed: %s", path)
	}

	// Resolve symbolic links for the prefix and suffix checks. The OpenInRoot and
	// O_NOFOLLOW below protect against TOCTOU attacks.
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %v", err)
	}

	if !strings.HasSuffix(allowedPrefix, "/") {
		allowedPrefix = allowedPrefix + "/"
	}

	var file *os.File
	if isLogFile(resolvedPath) {
		// Files ending with .log are allowed regardless of where they are
		// located in the file system, so we don't need to protect againt
		// symlink attacks for the components of the path.  For example, if the
		// path /var/log/foo/bar.log now points to /etc/bar.log (/var/log/foo ->
		// /etc), it's still a valid log file.
		//
		// We still do need to verify that the last component is still not a
		// symbolic link, O_NOFOLLOW ensures this.  For example, if
		// /var/log/foo/bar.log now points to /etc/shadow (bar.log ->
		// /etc/shadow), it should be prevented from being opened.
		file, err = os.OpenFile(resolvedPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	} else if strings.HasPrefix(resolvedPath, allowedPrefix) {
		// Files not ending with .log are only allowed if they are in
		// allowedPrefix.  OpenInRoot expects a path relative to the base
		// directory.
		relativePath := resolvedPath[len(allowedPrefix):]

		// OpenInRoot ensures that the path cannot escape the /var/log directory
		// (expanding symlinks, but protecting against symlink attacks).
		file, err = os.OpenInRoot(allowedPrefix, relativePath)
	} else {
		err = fmt.Errorf("non-log file not allowed")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", path, err)
	}

	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file %s: %v", path, err)
	}

	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("file %s is not a regular file", path)
	}

	if !isTextFile(file) {
		file.Close()
		return nil, errors.New("not a text file")
	}

	return file, nil
}

func validateAndOpen(path string) (*os.File, error) {
	return validateAndOpenWithPrefix(path, "/var/log/")
}
