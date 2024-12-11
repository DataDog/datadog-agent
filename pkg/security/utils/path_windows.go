// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"strings"

	"golang.org/x/sys/windows"
)

// NormalizePath normalize path
func NormalizePath(path string) string {
	return strings.TrimPrefix(path, "\\??\\")
}

// GetLongPathName converts the specified path to its long form
func GetLongPathName(path string) (string, error) {
	shortPath, err := windows.UTF16FromString(path)
	if err != nil {
		return "", err
	}

	longPath := make([]uint16, len(shortPath))
	n, err := windows.GetLongPathName(&shortPath[0], &longPath[0], uint32(len(longPath)))
	if err != nil {
		return "", err
	}

	if n > uint32(len(longPath)) {
		longPath = make([]uint16, n)
		_, err = windows.GetLongPathName(&shortPath[0], &longPath[0], uint32(len(longPath)))
		if err != nil {
			return "", err
		}
	}

	return windows.UTF16ToString(longPath), nil
}
