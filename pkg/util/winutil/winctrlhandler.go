// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import "syscall"

var (
	setConsoleCtrlHandler = k32.NewProc("SetConsoleCtrlHandler")
)

const (
	// CtrlCEvent is code for Cntrl+C event
	CtrlCEvent = 0
	// CtrlBreakEvent is code for Cntrl+Break event
	CtrlBreakEvent = 1
)

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetConsoleCtrlHandler sets the handler function for console control events.
func SetConsoleCtrlHandler(handler func(uint32) bool, add bool) error {
	ret, _, err := setConsoleCtrlHandler.Call(
		uintptr(syscall.NewCallbackCDecl(func(sig uint32) uintptr {
			if handler(sig) {
				return 1
			}
			return 0
		})),
		uintptr(boolToInt(add)))
	if ret == 0 {
		return err
	}
	return nil
}
