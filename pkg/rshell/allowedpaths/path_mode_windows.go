// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package allowedpaths

func resolveAllowedPathMode(path string) (string, pathMode) {
	stripped, mode, ok := splitAllowedPathMode(path)
	if !ok {
		return path, pathModeReadOnly
	}
	// Windows treats terminal ":ro" and ":rw" as rshell access-mode
	// metadata unconditionally. Unlike POSIX, where ":" is ordinary filename
	// text and an existing literal path must be preserved, Windows path
	// components cannot normally end this way without entering NTFS alternate
	// data stream syntax (for example, "file:rw"). We intentionally choose the
	// rshell mode suffix interpretation for this unsupported ambiguity.
	return stripped, mode
}
