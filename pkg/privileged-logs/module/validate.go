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

// validateAndOpenWithPrefix validates and opens a log file.
//
// When policy is common.FollowSymlinks (the default), symbolic links in path are
// resolved via filepath.EvalSymlinks before the allow-list check, and the resolved
// path is then re-opened with O_NOFOLLOW to close the TOCTOU window between
// resolution and the open.
//
// When policy is common.RejectSymlinks (used for process_log-discovered paths),
// the path is treated as already canonical: no EvalSymlinks call is made, and the
// path is opened directly with O_NOFOLLOW.  A symlink encountered at any component
// causes an immediate error.  This closes the residual TOCTOU race where an
// attacker could swap the file to a symlink pointing at an allow-listed root log
// in the gap between our O_NOFOLLOW open (which succeeds on the real file) and
// the module's EvalSymlinks call.
//
// The toctou parameter is a test-only hook called between EvalSymlinks and the
// O_NOFOLLOW open in the follow-symlinks path; pass nil in production.
func validateAndOpenWithPrefix(path, allowedPrefix string, policy common.SymlinkPolicy, toctou func()) (*os.File, error) {
	if path == "" {
		return nil, errors.New("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("relative path not allowed: %s", path)
	}

	var resolvedPath string

	if policy == common.RejectSymlinks {
		// In no-follow mode the caller guarantees the path is canonical.  Skip
		// EvalSymlinks entirely: open every component with O_NOFOLLOW so that a
		// symlink planted after the agent's discovery check causes an immediate
		// error rather than being followed.
		//
		// Normalize with filepath.Clean so that isAllowed and openPathWithoutSymlinks
		// see the same path.  Without this, a path like /var/log/../../etc/secret.log
		// would pass the isAllowed prefix check (string-prefix match on the raw path)
		// but openPathWithoutSymlinks would clean it to /etc/secret.log, bypassing
		// the allow-list.
		resolvedPath = filepath.Clean(path)
	} else {
		// Resolve symbolic links for the path and file name checks.
		var err error
		resolvedPath, err = filepath.EvalSymlinks(path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
		}

		// Callback for tests to change the filesystem after we called EvalSymlinks,
		// in order to simulate a TOCTOU attack.
		if toctou != nil {
			toctou()
		}
	}

	if !isAllowed(resolvedPath, allowedPrefix) {
		return nil, fmt.Errorf("non-log file not allowed: %s", resolvedPath)
	}

	// We use openPathWithoutSymlinks on the resolved path to verify each
	// component with O_NOFOLLOW to ensure that none of the path components
	// were replaced with symlinks after we called EvalSymlinks (follow mode),
	// or to enforce that the path has no symlinks at all (no-follow mode).
	file, err := openPathWithoutSymlinks(resolvedPath)
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

func validateAndOpen(path string, policy common.SymlinkPolicy) (*os.File, error) {
	return validateAndOpenWithPrefix(path, "/var/log/", policy, nil)
}
