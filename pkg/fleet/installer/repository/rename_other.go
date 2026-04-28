// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package repository

import "os"

// renamePackageDir is a plain os.Rename on non-Windows platforms.
// See rename_windows.go for the Windows implementation.
func renamePackageDir(sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
