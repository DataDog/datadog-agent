// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package utils

// GetFSTypeFromFilePath returns the filesystem type of the mount holding the speficied file path
func GetFSTypeFromFilePath(_ string) string {
	// not implemented yet for windows/macos
	return ""
}
