// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package etw

/*
#include "etw.h"
*/
import "C"
import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// StopETWSession stops an ETW trace session by name (e.g. an autologger session).
// Uses ControlTraceW with EVENT_TRACE_CONTROL_STOP.
func StopETWSession(sessionName string) error {
	utf16Name, err := windows.UTF16FromString(sessionName)
	if err != nil {
		return fmt.Errorf("invalid session name: %w", err)
	}

	ret := C.DDStopETWSession((*C.WCHAR)(unsafe.Pointer(&utf16Name[0])))
	switch status := windows.Errno(ret); status {
	case windows.ERROR_SUCCESS, windows.ERROR_MORE_DATA:
		return nil
	default:
		return status
	}
}
