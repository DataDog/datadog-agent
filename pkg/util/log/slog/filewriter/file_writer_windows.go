// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package filewriter

import "os"

// openLogDirFile opens the log file path as the writer opens its other log
// files. The Agent logs of Windows sit outside logDir, so it serves to build
// the package alone.
func openLogDirFile(path string, perm os.FileMode) (*os.File, bool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, perm)
	return f, false, err
}

// ownFile reports false: the writer leaves the access of Windows log files
// as it finds them.
func ownFile(os.FileInfo) bool {
	return false
}
