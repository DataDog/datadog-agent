// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package uptime

import (
	"time"

	"golang.org/x/sys/windows"
)

// For testing purpose
var uptime = calcUptime

var (
	modkernel = windows.NewLazyDLL("kernel32.dll")

	procGetTickCount64 = modkernel.NewProc("GetTickCount64")
)

func calcUptime() (uint64, error) {
	upTime := time.Duration(getTickCount64()) * time.Millisecond
	return uint64(upTime.Seconds()), nil
}

func getTickCount64() int64 {
	ret, _, _ := procGetTickCount64.Call()
	return int64(ret)
}
