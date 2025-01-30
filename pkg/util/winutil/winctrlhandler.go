// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import "golang.org/x/sys/windows"

var (
	setConsoleCtrlHandler = k32.NewProc("SetConsoleCtrlHandler")
)

// Console control signal constants
//
// https://learn.microsoft.com/en-us/windows/console/handlerroutine
const (
	CtrlCEvent     = 0
	CtrlBreakEvent = 1
)

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetConsoleCtrlHandler sets the handler function for console control events.
//
// https://learn.microsoft.com/en-us/windows/console/setconsolectrlhandler
func SetConsoleCtrlHandler(handler func(uint32) bool, add bool) error {
	ret, _, err := setConsoleCtrlHandler.Call(
		uintptr(windows.NewCallback(func(sig uint32) uintptr {
			return uintptr(boolToInt(handler(sig)))
		})),
		uintptr(boolToInt(add)))
	if ret == 0 {
		return err
	}
	return nil
}
