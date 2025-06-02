// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package runtime defines limits for the Go runtime
package runtime

import "golang.org/x/sys/unix"

// DisableTransparentHugePages disables transparent huge pages (THP) for the current process.
func DisableTransparentHugePages() error {
	return unix.Prctl(unix.PR_SET_THP_DISABLE, 1, 0, 0, 0)
}
