// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package sysinfo provides cross-platform access to system uptime and load
// average data. It reads kernel-provided values from fixed, non-user-controlled
// sources (/proc/uptime and /proc/loadavg on Linux; sysctl on Darwin;
// GetTickCount64 on Windows). These paths are hardcoded and never derived from
// shell-script input, so this package uses os.Open directly rather than routing
// through the AllowedPaths sandbox — the same intentional bypass used by
// diskstats, procnetroute, and procsyskernel.
package sysinfo

import "errors"

// ErrNotSupported is returned on platforms with no backend implementation.
var ErrNotSupported = errors.New("not supported on this platform")

// Info holds the data points read from the kernel.
type Info struct {
	// UptimeSeconds is the number of seconds elapsed since the last boot.
	UptimeSeconds float64

	// Load1, Load5, Load15 are the 1-, 5-, and 15-minute load averages.
	// These fields are only meaningful when LoadAvailable is true.
	Load1, Load5, Load15 float64

	// LoadAvailable reports whether load average data is available on this
	// platform. It is false on Windows, which has no native load-average API.
	LoadAvailable bool

	// BootTime is the Unix epoch second of the last system boot.
	// It is computed as time.Now().Unix() - int64(UptimeSeconds), so it may
	// be off by up to one second from the true boot time.
	BootTime int64
}

// Get reads uptime and load-average data from the kernel.
// The implementation is platform-specific; see the build-tagged files.
func Get() (Info, error) {
	return getImpl()
}
