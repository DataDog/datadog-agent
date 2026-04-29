// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package paths

import (
	"context"
	"os"
)

// Rename is a plain os.Rename on non-Windows platforms.
// See rename_windows.go for the Windows implementation, which retries on
// transient access-denied / sharing-violation errors.
func Rename(_ context.Context, sourcePath, targetPath string) error {
	return os.Rename(sourcePath, targetPath)
}
