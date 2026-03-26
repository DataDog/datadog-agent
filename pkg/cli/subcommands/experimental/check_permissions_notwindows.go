// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package experimental

import (
	"fmt"
	"os"
)

// checkFilePermissions warns if the config file is world-readable (mode bits
// accessible to other/world). Returns (true, nil) if permissions look good,
// or (false, error) with context if they don't.
func checkFilePermissions(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("cannot stat config file: %w", err)
	}
	if info.Mode()&0o007 != 0 {
		return false, fmt.Errorf("config file is world-readable (mode %s) — API key may be exposed. Fix: chmod 640 %s", info.Mode(), path)
	}
	return true, nil
}
