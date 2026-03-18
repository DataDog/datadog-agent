// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package heuristic

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// parentProcessUptimeMS returns the parent process uptime in milliseconds.
// It reads /proc/<ppid>/stat field 22 (starttime in jiffies) and /proc/uptime.
func parentProcessUptimeMS() (int64, bool) {
	ppid := syscall.Getppid()
	if ppid <= 0 {
		return 0, false
	}

	// Read system uptime from /proc/uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return 0, false
	}
	systemUptimeSecs, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return 0, false
	}

	// Read parent process start time from /proc/<ppid>/stat field 22 (0-indexed: index 21)
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", ppid))
	if err != nil {
		return 0, false
	}
	statFields := strings.Fields(string(statData))
	if len(statFields) < 22 {
		return 0, false
	}
	startTimeJiffies, err := strconv.ParseInt(statFields[21], 10, 64)
	if err != nil {
		return 0, false
	}

	// Convert jiffies to seconds (clock ticks per second = 100 on nearly all Linux systems)
	const clockTicks = 100
	processStartSecs := float64(startTimeJiffies) / clockTicks
	processUptimeSecs := systemUptimeSecs - processStartSecs
	if processUptimeSecs < 0 {
		processUptimeSecs = 0
	}

	return int64(processUptimeSecs * 1000), true
}
