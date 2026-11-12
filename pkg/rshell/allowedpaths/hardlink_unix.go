// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"errors"
	"io/fs"
	"os"
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

// errLinkCountUnavailable reports that the platform stat structure carried no
// link count, so inode aliasing cannot be ruled out.
var errLinkCountUnavailable = errors.New("cannot determine hard link count")

// openFileLinkCount returns the link count of an already-open file from the
// FileInfo its descriptor's fstat(2) produced. It fails rather than guessing
// when the count is unavailable, so write guards fail closed.
func openFileLinkCount(_ *os.File, info fs.FileInfo) (uint64, error) {
	links, ok := fileLinkCount(info)
	if !ok {
		return 0, errLinkCountUnavailable
	}
	return links, nil
}
