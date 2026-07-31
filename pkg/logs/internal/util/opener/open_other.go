// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// OpenLogFile opens a file with filesystem.OpenShared.
// On non-Linux platforms we don't need to support symlink rejection since it's
// only needed for process_log-discovered paths which are currently only
// supported on Linux.
func OpenLogFile(path string) (*os.File, error) {
	return filesystem.OpenShared(path)
}

// OpenLogFileNoFollow falls back to a regular open on non-Linux platforms:
// symlink rejection is only needed for process_log-discovered paths, and
// process_log discovery (based on /proc/<pid>/fd) is Linux-only, so this path
// is not reachable with an untrusted, attacker-controlled symlink swap here.
func OpenLogFileNoFollow(path string) (*os.File, error) {
	return filesystem.OpenShared(path)
}

// StatLogFile stats a log file
func StatLogFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
