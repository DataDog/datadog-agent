// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filesystem

import (
	"os"
	"time"
)

// GetFileSize gets the file size
func GetFileSize(path string) (int64, error) {
	stat, err := os.Stat(path)

	if err != nil {
		return 0, err
	}

	return stat.Size(), nil
}

// GetFileModTime gets the modification time
func GetFileModTime(path string) (time.Time, error) {
	stat, err := os.Stat(path)

	if err != nil {
		return time.Time{}, err
	}

	return stat.ModTime(), nil
}
