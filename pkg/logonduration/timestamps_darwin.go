// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package logonduration

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework OSLog

#include <stdlib.h>

// Returns Unix timestamp (seconds since epoch) or 0 on error
// fileVaultEnabled: 1 = FileVault enabled, 0 = FileVault disabled
double queryLoginWindowTimestamp(double bootTimestamp, int fileVaultEnabled);

// Returns Unix timestamp (seconds since epoch) or 0 on error
double queryLoginTimestamp(double bootTimestamp);

// Returns 1 if FileVault is enabled, 0 if disabled, -1 on error
int checkFileVaultEnabled(void);

// Returns Unix timestamp (seconds since epoch) or 0 on error
double queryDesktopReadyTimestamp(double bootTimestamp);
*/
import "C"

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetBootTime returns the system boot time using sysctl kern.boottime
func GetBootTime() (time.Time, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return time.Time{}, fmt.Errorf("sysctl kern.boottime failed: %w", err)
	}
	return time.Unix(tv.Sec, int64(tv.Usec)*1000), nil
}

// GetLoginWindowTime queries OSLogStore for when the login window appeared.
// The query differs based on whether FileVault is enabled.
// This requires root privileges to access the local log store.
func GetLoginWindowTime(bootTime time.Time, fileVaultEnabled bool) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	fvEnabled := C.int(0)
	if fileVaultEnabled {
		fvEnabled = 1
	}
	result := C.queryLoginWindowTimestamp(bootTimestamp, fvEnabled)

	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login window time from unified logs")
	}

	resultFloat := float64(result)
	return time.Unix(int64(resultFloat), int64((resultFloat-float64(int64(resultFloat)))*1e9)), nil
}

// GetLoginTime queries OSLogStore for when the user completed login.
// This works the same way with or without FileVault.
// This requires root privileges to access the local log store.
func GetLoginTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryLoginTimestamp(bootTimestamp)

	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query login time from unified logs")
	}

	resultFloat := float64(result)
	return time.Unix(int64(resultFloat), int64((resultFloat-float64(int64(resultFloat)))*1e9)), nil
}

// GetDesktopReadyTime queries OSLogStore for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// This requires root privileges to access the local log store.
func GetDesktopReadyTime(bootTime time.Time) (time.Time, error) {
	bootTimestamp := C.double(float64(bootTime.Unix()))
	result := C.queryDesktopReadyTimestamp(bootTimestamp)

	if result == 0 {
		return time.Time{}, fmt.Errorf("failed to query desktop ready time from unified logs")
	}

	resultFloat := float64(result)
	return time.Unix(int64(resultFloat), int64((resultFloat-float64(int64(resultFloat)))*1e9)), nil
}

// IsFileVaultEnabled checks if FileVault is enabled.
// This requires root privileges to run fdesetup.
func IsFileVaultEnabled() (bool, error) {
	result := C.checkFileVaultEnabled()
	if result < 0 {
		return false, fmt.Errorf("failed to check FileVault status")
	}
	return result == 1, nil
}

// GetLoginTimestamps collects all login-related timestamps from the system.
// This is the main entry point for the system-probe module.
func GetLoginTimestamps() *LoginTimestamps {
	result := &LoginTimestamps{}

	// Get boot time first (needed as reference for log queries)
	bootTime, err := GetBootTime()
	if err != nil {
		result.Error = fmt.Sprintf("failed to get boot time: %v", err)
		return result
	}

	// Check FileVault status first (needed for login window query)
	start := time.Now()
	fileVaultEnabled := false
	if fv, err := IsFileVaultEnabled(); err == nil {
		result.FileVaultEnabled = &fv
		fileVaultEnabled = fv
		log.Infof("logonduration: FileVault enabled: %v (query took %.3fs)", fv, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to check FileVault status: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get login window time via CGO to OSLogStore (query depends on FileVault status)
	start = time.Now()
	if lwt, err := GetLoginWindowTime(bootTime, fileVaultEnabled); err == nil {
		result.LoginWindowTime = &lwt
		log.Infof("logonduration: login window time: %v (query took %.3fs)", lwt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login window time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get login time via CGO to OSLogStore
	start = time.Now()
	if lt, err := GetLoginTime(bootTime); err == nil {
		result.LoginTime = &lt
		log.Infof("logonduration: login time: %v (query took %.3fs)", lt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get desktop ready time via CGO to OSLogStore (Dock checkin with launchservicesd)
	start = time.Now()
	if drt, err := GetDesktopReadyTime(bootTime); err == nil {
		result.DesktopReadyTime = &drt
		log.Infof("logonduration: desktop ready time: %v (query took %.3fs)", drt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get desktop ready time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	return result
}
