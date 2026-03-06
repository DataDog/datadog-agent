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
#include "timestamps_darwin.h"
*/
import "C"

import (
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// cTimestampToTime converts a C double Unix timestamp to time.Time with nanosecond precision.
func cTimestampToTime(result C.double) time.Time {
	resultFloat := float64(result)
	sec := int64(resultFloat)
	nsec := int64((resultFloat - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// GetLoginWindowTime queries OSLogStore for when the login window appeared.
// The query differs based on whether FileVault is enabled.
// This requires root privileges to access the local log store.
func GetLoginWindowTime(fileVaultEnabled bool) (time.Time, error) {
	fvEnabled := C.int(0)
	if fileVaultEnabled {
		fvEnabled = 1
	}
	result := C.queryLoginWindowTimestamp(fvEnabled)
	if result == 0 {
		return time.Time{}, errors.New("failed to query login window time from unified logs")
	}
	return cTimestampToTime(result), nil
}

// GetLoginTime queries OSLogStore for when the user completed login.
// This works the same way with or without FileVault.
// This requires root privileges to access the local log store.
func GetLoginTime() (time.Time, error) {
	result := C.queryLoginTimestamp()
	if result == 0 {
		return time.Time{}, errors.New("failed to query login time from unified logs")
	}
	return cTimestampToTime(result), nil
}

// GetDesktopReadyTime queries OSLogStore for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// This requires root privileges to access the local log store.
func GetDesktopReadyTime() (time.Time, error) {
	result := C.queryDesktopReadyTimestamp()
	if result == 0 {
		return time.Time{}, errors.New("failed to query desktop ready time from unified logs")
	}
	return cTimestampToTime(result), nil
}

// IsFileVaultEnabled checks if FileVault is enabled.
// This requires root privileges to run fdesetup.
func IsFileVaultEnabled() (bool, error) {
	result := C.checkFileVaultEnabled()
	if result < 0 {
		return false, errors.New("failed to check FileVault status")
	}
	return result == 1, nil
}

// GetLoginTimestamps collects all login-related timestamps from the system.
// This is the main entry point for the system-probe module.
func GetLoginTimestamps() LoginTimestamps {
	result := LoginTimestamps{}

	// Check FileVault status first (needed for login window query)
	start := time.Now()
	if fv, err := IsFileVaultEnabled(); err == nil {
		result.FileVaultEnabled = fv
		log.Infof("logonduration: FileVault enabled: %v (query took %.3fs)", fv, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to check FileVault status: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get login window time via CGO to OSLogStore (query depends on FileVault status)
	start = time.Now()
	if lwt, err := GetLoginWindowTime(result.FileVaultEnabled); err == nil {
		result.LoginWindowTime = lwt
		log.Infof("logonduration: login window time: %v (query took %.3fs)", lwt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login window time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get login time via CGO to OSLogStore
	start = time.Now()
	if lt, err := GetLoginTime(); err == nil {
		result.LoginTime = lt
		log.Infof("logonduration: login time: %v (query took %.3fs)", lt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get login time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	// Get desktop ready time via CGO to OSLogStore (Dock checkin with launchservicesd)
	start = time.Now()
	if drt, err := GetDesktopReadyTime(); err == nil {
		result.DesktopReadyTime = drt
		log.Infof("logonduration: desktop ready time: %v (query took %.3fs)", drt, time.Since(start).Seconds())
	} else {
		log.Warnf("logonduration: failed to get desktop ready time: %v (query took %.3fs)", err, time.Since(start).Seconds())
	}

	return result
}
