// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package winpoll provides a non-consuming readability probe for Windows
// file handles, used by the `read` builtin to implement bash's `read -t 0`
// semantics on platforms that lack the Unix poll(2) syscall.
//
// This file contains a narrow unsafe exception: unsafe.Pointer is used
// solely to pass &avail to PeekNamedPipe via kernel32.dll. No pointer
// arithmetic occurs after the DLL call — the returned uint32 is consumed
// directly by the caller.
package winpoll

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = syscall.MustLoadDLL("kernel32.dll")
	peekNamedPipeProc = kernel32.MustFindProc("PeekNamedPipe")
)

// PollNonConsuming reports whether reading from the given file descriptor
// would yield at least one byte of buffered data without consuming any.
// Mirrors the Unix `pollInputNonConsuming` contract:
//
//	supported = false → caller should fall back conservatively (Code 1).
//	supported = true,  available = false → no data buffered, would block.
//	supported = true,  available = true  → data is buffered, read() will
//	                                       return immediately with bytes.
//
// The implementation dispatches by Windows file type (`GetFileType`):
//
//   - FILE_TYPE_DISK (regular file): always available — file reads don't
//     block on Windows. Reported as supported & available.
//   - FILE_TYPE_PIPE (anonymous or named pipe): non-consuming probe via
//     PeekNamedPipe; lpTotalBytesAvail reports buffered byte count.
//   - FILE_TYPE_CHAR (console / TTY): GetNumberOfConsoleInputEvents
//     reports queued input-record count. Note: this counts ALL event
//     records (key press, key release, mouse, focus, window, menu),
//     not just readable characters — so a key release alone shows
//     available, even though a synchronous Read would still block
//     waiting for a complete line. This is the closest approximation
//     available without a full PeekConsoleInput parse; bash on
//     Windows-WSL uses similar heuristics.
//   - Anything else (sockets, unknown): unsupported — caller falls back
//     to the conservative "not available" answer.
func PollNonConsuming(fd uintptr) (available, supported bool) {
	h := windows.Handle(fd)
	typ, err := windows.GetFileType(h)
	if err != nil {
		return false, false
	}
	// FILE_TYPE_REMOTE is an OR'd modifier bit (Windows reserves it for
	// remote-mounted volumes); strip it before the type switch.
	switch typ &^ windows.FILE_TYPE_REMOTE {
	case windows.FILE_TYPE_DISK:
		return true, true

	case windows.FILE_TYPE_PIPE:
		var avail uint32
		// PeekNamedPipe(hNamedPipe, lpBuffer=NULL, nBufferSize=0,
		//               lpBytesRead=NULL, lpTotalBytesAvail=&avail,
		//               lpBytesLeftThisMessage=NULL)
		// Returns BOOL: nonzero on success. With NULL lpBuffer the
		// call only reports counts and never consumes data.
		r1, _, callErr := peekNamedPipeProc.Call(
			uintptr(h),                      // hNamedPipe
			0,                               // lpBuffer
			0,                               // nBufferSize
			0,                               // lpBytesRead
			uintptr(unsafe.Pointer(&avail)), //nolint:govet // narrow DLL exception
			0,                               // lpBytesLeftThisMessage
		)
		if r1 == 0 {
			// PeekNamedPipe failure. ERROR_BROKEN_PIPE means the
			// writer end has closed — the pipe is EOF-ready, which
			// matches the Unix POLLHUP semantics: bash 5.2 returns
			// rc=0 for `read -t 0` on a drained closed pipe because
			// the next Read would complete immediately with EOF
			// (no blocking). Treat as available so cross-platform
			// behaviour stays consistent for `printf '' | read -t 0`.
			if errno, ok := callErr.(syscall.Errno); ok && errno == windows.ERROR_BROKEN_PIPE {
				return true, true
			}
			return false, false
		}
		return avail > 0, true

	case windows.FILE_TYPE_CHAR:
		var n uint32
		if err := windows.GetNumberOfConsoleInputEvents(h, &n); err != nil {
			return false, false
		}
		return n > 0, true

	default:
		return false, false
	}
}
