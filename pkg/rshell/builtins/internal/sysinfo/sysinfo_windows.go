// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package sysinfo

import (
	"syscall"
	"time"
)

var (
	kernel32       = syscall.MustLoadDLL("kernel32.dll")
	getTickCount64 = kernel32.MustFindProc("GetTickCount64")
)

func getImpl() (Info, error) {
	// GetTickCount64 returns milliseconds since last boot as a ULONGLONG.
	// It always succeeds; the third return from Call (GetLastError) is not
	// meaningful here, so it is discarded.
	// ticksToMs is build-tagged: on 386 it combines lo and hi to reconstruct
	// the full 64-bit value; on 64-bit targets lo already holds the full value.
	lo, hi, _ := getTickCount64.Call()
	ms := ticksToMs(lo, hi)

	uptimeSecs := float64(ms) / 1000.0
	return Info{
		UptimeSeconds: uptimeSecs,
		LoadAvailable: false,
		BootTime:      time.Now().Unix() - int64(uptimeSecs),
	}, nil
}
