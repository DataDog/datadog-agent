// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filewriter

import (
	"errors"
	"os"
	"syscall"
)

// openLogDirFile opens the log file path under logDir and reports whether it
// opened path itself. The users who can write to logDir can replace a log
// file there with a symlink, so the writer writes through a symlink only to
// a file that exists, such as /dev/null, and leaves the mode and group of
// that file as they are.
func openLogDirFile(path string, perm os.FileMode) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, perm)
	if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EMLINK) {
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		return f, false, err
	}
	return f, err == nil, err
}

// ownFile reports whether fi describes a file of the writer's user that has
// no other name, so that a change of its mode or group reaches the log file
// alone.
func ownFile(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1 && int(st.Uid) == os.Geteuid()
}
