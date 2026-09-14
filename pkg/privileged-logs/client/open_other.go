// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"errors"
	"os"
	"runtime"
)

// Open provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	// On AIX, opening a directory like a regular file works,
	// so we need to explicitly check that the file is not a directory
	if runtime.GOOS == "aix" {
		stat, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}

		if stat.IsDir() {
			_ = file.Close()
			return nil, errors.New("file is a directory")
		}
	}

	return file, nil
}

// Stat provides a fallback for non-Linux platforms where the privileged logs module is not available.
func Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
