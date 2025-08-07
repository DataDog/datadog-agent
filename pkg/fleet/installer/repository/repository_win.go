// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package repository

import "os"

// copyFileWithPermissions copies a file from src to dst with the same permissions.
func copyFileWithPermissions(src, dst string, _ os.FileInfo) error {
	return copyFile(src, dst)
}
