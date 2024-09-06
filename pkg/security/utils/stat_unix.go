// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import (
	"io/fs"
	"syscall"
)

// UnixStat is an unix only equivalent to os.Stat, but alloc-free,
// and returning directly the platform-specific syscall.Stat_t structure.
func UnixStat(path string) (syscall.Stat_t, error) {
	var stat syscall.Stat_t
	var err error
	for {
		err := syscall.Stat(path, &stat)
		if err != syscall.EINTR {
			break
		}
	}
	return stat, err
}

// UnixStatModeToGoFileMode converts a Unix mode to a Go fs.FileMode.
func UnixStatModeToGoFileMode(mode uint32) fs.FileMode {
	fsmode := fs.FileMode(mode & 0777)
	switch mode & syscall.S_IFMT {
	case syscall.S_IFBLK:
		fsmode |= fs.ModeDevice
	case syscall.S_IFCHR:
		fsmode |= fs.ModeDevice | fs.ModeCharDevice
	case syscall.S_IFDIR:
		fsmode |= fs.ModeDir
	case syscall.S_IFIFO:
		fsmode |= fs.ModeNamedPipe
	case syscall.S_IFLNK:
		fsmode |= fs.ModeSymlink
	case syscall.S_IFREG:
		// nothing to do
	case syscall.S_IFSOCK:
		fsmode |= fs.ModeSocket
	}
	if mode&syscall.S_ISGID != 0 {
		fsmode |= fs.ModeSetgid
	}
	if mode&syscall.S_ISUID != 0 {
		fsmode |= fs.ModeSetuid
	}
	if mode&syscall.S_ISVTX != 0 {
		fsmode |= fs.ModeSticky
	}
	return fsmode
}
