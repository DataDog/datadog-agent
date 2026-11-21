// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package logondurationimpl

import (
	"fmt"
	"os"
	"syscall"
)

// consoleUserLoggedIn reports whether a non-root user owns the screen's console.
func consoleUserLoggedIn() (bool, error) {
	info, err := os.Stat("/dev/console")
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("unexpected /dev/console stat type %T", info.Sys())
	}
	return stat.Uid != 0, nil
}
