// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package opener provides utilities to open log files with appropriate permissions.
package opener

import (
	"fmt"
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

// OpenLogFileNoFollow is not supported on non-Linux platforms.
func OpenLogFileNoFollow(path string) (*os.File, error) {
	return nil, fmt.Errorf("opener: no-follow open is not supported on non-Linux platforms: %s", path)
}

// StatLogFile stats a log file
func StatLogFile(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
