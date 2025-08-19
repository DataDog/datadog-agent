// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.
//go:build windows

package winutil

import (
	"syscall"
	"unsafe"
)

// IsProcessElevated opens the process token and checks elevation status,
// returning true if the process is elevated and false if not elevated.
func IsProcessElevated() (bool, error) {
	p, e := syscall.GetCurrentProcess()
	if e != nil {
		return false, e
	}
	var t syscall.Token
	e = syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &t)
	if e != nil {
		return false, e
	}
	defer syscall.CloseHandle(syscall.Handle(t))

	var elevated uint32
	n := uint32(unsafe.Sizeof(elevated))
	for {
		b := make([]byte, n)
		e := syscall.GetTokenInformation(t, syscall.TokenElevation, &b[0], uint32(len(b)), &n)
		if e == nil {
			elevated = *(*uint32)(unsafe.Pointer(&b[0]))
			return elevated != 0, nil
		}
		if e != syscall.ERROR_INSUFFICIENT_BUFFER {
			return false, e
		}
		if n <= uint32(len(b)) {
			return false, e
		}
	}
}
