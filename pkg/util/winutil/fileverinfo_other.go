// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package winutil

// FileVersionInfo is a placeholder on non-Windows platforms.
type FileVersionInfo struct {
	CompanyName      string
	ProductName      string
	FileVersion      string
	ProductVersion   string
	OriginalFilename string
	InternalName     string
}

// GetFileVersionInfoStrings is not implemented on non-Windows platforms.
func GetFileVersionInfoStrings(_ string) (FileVersionInfo, error) {
	return FileVersionInfo{}, nil
}
 
