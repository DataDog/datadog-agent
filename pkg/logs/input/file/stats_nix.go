// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package file

import (
	"os"
	"syscall"
	"time"
)

// Ctime returns the creation time of the file.
func Ctime(filePath string) (time.Time, error) {
	f, err := os.Stat(filePath)
	if err != nil {
		return time.Time{}, err
	}
	stat := f.Sys().(*syscall.Stat_t)
	return time.Unix(stat.Ctim.Unix()), nil
}
