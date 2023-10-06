// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import "os"

// OpenShared reimplements the os.Open function for Windows because the default
// implementation opens files without the FILE_SHARE_DELETE flag.
// cf: https://github.com/golang/go/blob/release-branch.go1.11/src/syscall/syscall_windows.go#L271
// Without FILE_SHARE_DELETE, other users cannot rename/remove the file while
// this handle is open. Adding this flag allows the agent to have the file open,
// while not preventing it from being rotated/deleted.
//
// On non-Windows platforms, this calls through to os.Open directly.
func OpenShared(path string) (*os.File, error) {
	return os.Open(path)
}
