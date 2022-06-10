// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package filesystem

import "os"

// OpenShared reimplements the os.Open function for Windows because the default
// implementation opens files without the FILE_SHARE_DELETE flag.
// cf: https://github.com/golang/go/blob/release-branch.go1.11/src/syscall/syscall_windows.go#L271
// This prevents users from moving/removing files when a log tailer is reading the file.
func OpenShared(path string) (*os.File, error) {
	return os.Open(path)
}
