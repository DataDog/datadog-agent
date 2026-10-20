// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"errors"
	"os"
)

func resolveAllowedPathMode(path string) (string, pathMode) {
	stripped, mode, ok := splitAllowedPathMode(path)
	if !ok {
		return path, pathModeReadOnly
	}
	// On POSIX filesystems, paths may literally end in ":ro" or ":rw".
	// Preserve an existing literal path, or any path we cannot prove is absent,
	// so a config suffix never widens access by stripping real filename text.
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return path, pathModeReadOnly
	}
	return stripped, mode
}
