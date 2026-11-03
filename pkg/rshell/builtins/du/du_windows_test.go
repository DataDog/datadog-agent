// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package du_test

import "os"

// canSymlink reports whether the test environment can create symbolic
// links. On Windows this requires Developer Mode or SeCreateSymbolicLink
// privilege, so probe by trying to make one.
func canSymlink() bool {
	tmp, err := os.MkdirTemp("", "du-symlink-probe")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)
	if err := os.Symlink("target", tmp+"/probe"); err != nil {
		return false
	}
	return true
}
