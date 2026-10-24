// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"io/fs"
	"syscall"
)

// fileLinkCount returns the hard-link count reported by fstat(2) for info.
// The second result is false when the platform stat structure is not
// available, in which case callers must not draw any conclusion about
// inode aliasing.
func fileLinkCount(info fs.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, false
	}
	return uint64(stat.Nlink), true
}
