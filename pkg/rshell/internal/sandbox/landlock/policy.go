// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package landlock applies a process-wide Landlock filesystem policy derived
// from rshell AllowedPaths entries.
//
// Restrict is irreversible. Callers must invoke it only in a disposable
// one-shot worker, after opening the file descriptors the worker needs and
// before running untrusted work.
package landlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupported indicates that Landlock cannot be applied on this platform
// or that the running Linux kernel cannot enforce the complete policy. A
// privileged worker must treat this as a hard failure.
var ErrUnsupported = errors.New("Landlock filesystem sandbox is unsupported")

// TrustedPathKind distinguishes a directory hierarchy from one exact file.
// Backend AllowedPaths remain directory hierarchies; this type is only for
// command-dependent paths which trusted in-process builtins open directly.
type TrustedPathKind uint8

const (
	// TrustedPathDirectory grants access beneath a directory hierarchy.
	TrustedPathDirectory TrustedPathKind = iota
	// TrustedPathFile grants access to one exact regular or pseudo file.
	TrustedPathFile
)

// TrustedPathAccess describes the narrow operations granted to a trusted
// command-dependent path.
type TrustedPathAccess uint8

const (
	// TrustedPathReadOnly grants file reads and, for directories, listing.
	TrustedPathReadOnly TrustedPathAccess = iota
	// TrustedPathReadRemoveFiles additionally permits removing files beneath a
	// directory. It is intended for an independently authorized journal vacuum.
	TrustedPathReadRemoveFiles
)

// TrustedPath is a command-dependent filesystem exception for an in-process
// builtin. It must be derived from the command being dispatched, not accepted
// as unsigned worker input.
//
// Optional may only be used for a known path whose absence is expected, such
// as one of systemd's alternative journal directories. Other open errors still
// fail closed.
type TrustedPath struct {
	Path     string
	Kind     TrustedPathKind
	Access   TrustedPathAccess
	Optional bool
}

type accessMode uint8

const (
	accessReadOnly accessMode = iota
	accessReadWrite
)

type pathRule struct {
	path string
	mode accessMode
}

func parseAllowedPaths(allowedPaths []string) ([]pathRule, error) {
	rules := make([]pathRule, 0, len(allowedPaths))
	for _, configuredPath := range allowedPaths {
		path, mode, err := parseAllowedPath(configuredPath)
		if err != nil {
			return nil, err
		}
		rules = append(rules, pathRule{path: path, mode: mode})
	}
	return rules, nil
}

// parseAllowedPath matches the interpreter's POSIX :ro/:rw suffix behavior.
// If a literal suffixed path already exists (or cannot be proven absent), the
// suffix is part of the path and the entry remains read-only. The Linux
// implementation subsequently opens the selected target once with O_PATH and
// uses that same descriptor for validation and Landlock rule creation.
func parseAllowedPath(configuredPath string) (string, accessMode, error) {
	if configuredPath == "" {
		return "", accessReadOnly, errors.New("Landlock AllowedPaths entry must not be empty")
	}
	if !filepath.IsAbs(configuredPath) {
		return "", accessReadOnly, fmt.Errorf("Landlock AllowedPaths entry %q must be absolute", configuredPath)
	}

	for _, suffix := range []struct {
		text string
		mode accessMode
	}{
		{text: ":ro", mode: accessReadOnly},
		{text: ":rw", mode: accessReadWrite},
	} {
		if !strings.HasSuffix(configuredPath, suffix.text) || len(configuredPath) <= len(suffix.text) {
			continue
		}
		if _, err := os.Lstat(configuredPath); err == nil || !errors.Is(err, os.ErrNotExist) {
			return filepath.Clean(configuredPath), accessReadOnly, nil
		}
		return filepath.Clean(strings.TrimSuffix(configuredPath, suffix.text)), suffix.mode, nil
	}

	return filepath.Clean(configuredPath), accessReadOnly, nil
}
