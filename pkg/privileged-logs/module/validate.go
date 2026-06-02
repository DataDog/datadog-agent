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

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
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
	resolvedPath, err := resolveFollowPath(path)
	if err != nil {
		return nil, err
	}

	// Callback for tests to change the filesystem after we called EvalSymlinks,
	// in order to simulate a TOCTOU attack.
	if toctou != nil {
		toctou()
	}

	return validateResolvedAndOpen(resolvedPath, allowedPrefix)
}

func validateAndOpenNoFollowWithPrefix(path, allowedPrefix string) (*os.File, error) {
	resolvedPath, err := resolveNoFollowPath(path)
	if err != nil {
		return nil, err
	}
	return validateResolvedAndOpen(resolvedPath, allowedPrefix)
}

func resolveFollowPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("relative path not allowed: %s", path)
	}

	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %s: %w", path, err)
	}
	return resolvedPath, nil
}

func resolveNoFollowPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("relative path not allowed: %s", path)
	}

	// In no-follow mode the caller guarantees the path is canonical. Skip
	// EvalSymlinks entirely: open every component with O_NOFOLLOW so that a
	// symlink planted after the agent's discovery check causes an immediate error.
	return filepath.Clean(path), nil
}

func validateResolvedAndOpen(resolvedPath, allowedPrefix string) (*os.File, error) {
	if !isAllowed(resolvedPath, allowedPrefix) {
		return nil, fmt.Errorf("non-log file not allowed: %s", resolvedPath)
	}

	// We use openPathWithoutSymlinks on the resolved path to verify each
	// component with O_NOFOLLOW to ensure that none of the path components
	// were replaced with symlinks after we called EvalSymlinks (follow mode),
	// or to enforce that the path has no symlinks at all (no-follow mode).
	file, err := common.OpenPathWithoutSymlinks(resolvedPath)
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

func validateAndOpenNoFollow(path string) (*os.File, error) {
	return validateAndOpenNoFollowWithPrefix(path, "/var/log/")
}
