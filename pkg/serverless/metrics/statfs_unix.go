// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package metrics

import (
	"golang.org/x/sys/unix"
)

func statfs(path string) (float64, float64, float64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return float64(stat.Bsize), float64(stat.Blocks), float64(stat.Bavail), err
}
