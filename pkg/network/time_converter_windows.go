// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

package network

import (
	"time"

	"golang.org/x/sys/windows"
)

var (
	modkernel          = windows.NewLazyDLL("kernel32.dll")
	procGetTickCount64 = modkernel.NewProc("GetTickCount64")
)

// getTickCount64() returns the time, in milliseconds, that have elapsed since
// the system was started
func getTickCount64() int64 {
	ret, _, _ := procGetTickCount64.Call()
	return int64(ret)
}

func driverTimeToUnixTime(t uint64) uint64 {
	ticks := getTickCount64()
	// GetTickCount64() returns the number of milliseconds that have elapsed since the system was started

	// t is the number of microseconds since the system was started.

	// convert this to a unix timestamp

	bootTime := time.Now().Add(-time.Duration(ticks) * time.Millisecond)
	opTime := bootTime.Add(time.Duration(t) * time.Microsecond)
	return uint64(opTime.UnixNano())

}
