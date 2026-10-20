// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package writeopen

import "path/filepath"

// HasTrailingDirSyntax reports whether relPath, as literally passed to
// Unlink, syntactically requires its target to resolve as a directory: it
// ends in a path separator, or its final component is "." or "..". POSIX/GNU
// tools reject such operands (e.g. "file/", "file/.") with ENOTDIR when the
// target — after following any symlink, since a trailing separator forces
// dereference — is not a directory, rather than silently operating on
// whatever remains once path cleaning drops the trailing syntax. "/" is
// always checked since it is the shell's own path-separator syntax; "\" is
// only a separator on Windows — on Unix it is a valid filename character.
//
// Exported so Sandbox.Remove can detect this syntax on the caller's raw path
// before toAbs's filepath.Join cleans the trailing separator away, and
// re-encode the requirement onto relPath before calling Unlink.
func HasTrailingDirSyntax(relPath string) bool {
	if relPath == "" {
		return false
	}
	last := relPath[len(relPath)-1]
	if last == '/' || (filepath.Separator == '\\' && last == '\\') {
		return true
	}
	base := filepath.Base(relPath)
	return base == "." || base == ".."
}
