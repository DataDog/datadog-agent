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
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// unixFloatToTime converts a float64 Unix timestamp to time.Time with nanosecond precision.
func unixFloatToTime(t float64) time.Time {
	sec := int64(t)
	nsec := int64((t - float64(sec)) * 1e9)
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
	var result C.double
	if errMsg := C.queryLoginWindowTimestamp(fvEnabled, &result); errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
		return time.Time{}, errors.New(C.GoString(errMsg))
	}
	return unixFloatToTime(float64(result)), nil
}

// GetLoginTime queries OSLogStore for when the user completed login.
// This works the same way with or without FileVault.
// This requires root privileges to access the local log store.
func GetLoginTime() (time.Time, error) {
	var result C.double
	if errMsg := C.queryLoginTimestamp(&result); errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
		return time.Time{}, errors.New(C.GoString(errMsg))
	}
	return unixFloatToTime(float64(result)), nil
}

// GetDesktopReadyTime queries OSLogStore for when the Dock checked in with launchservicesd.
// This indicates the desktop is ready for user interaction.
// This requires root privileges to access the local log store.
func GetDesktopReadyTime() (time.Time, error) {
	var result C.double
	if errMsg := C.queryDesktopReadyTimestamp(&result); errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
		return time.Time{}, errors.New(C.GoString(errMsg))
	}
	return unixFloatToTime(float64(result)), nil
}

// IsFileVaultEnabled checks if FileVault is enabled.
// This requires root privileges to run fdesetup.
func IsFileVaultEnabled() (bool, error) {
	var result C.int
	if errMsg := C.checkFileVaultEnabled(&result); errMsg != nil {
		defer C.free(unsafe.Pointer(errMsg))
		return false, errors.New(C.GoString(errMsg))
	}
	return result == 1, nil
}

// GetLoginTimestamps collects all login-related timestamps from the system.
// This is the main entry point for the system-probe module.
func GetLoginTimestamps() LoginTimestamps {
	result := LoginTimestamps{}

	// Check FileVault status first (needed for login window query)
	if fv, err := IsFileVaultEnabled(); err == nil {
		result.FileVaultEnabled = fv
	} else {
		log.Warnf("logonduration: failed to check FileVault status: %v", err)
	}

	// Get login window time via CGO to OSLogStore (query depends on FileVault status)
	if lwt, err := GetLoginWindowTime(result.FileVaultEnabled); err == nil {
		result.LoginWindowTime = lwt
	} else {
		log.Warnf("logonduration: failed to get login window time: %v", err)
	}

	// Get login time via CGO to OSLogStore
	if lt, err := GetLoginTime(); err == nil {
		result.LoginTime = lt
	} else {
		log.Warnf("logonduration: failed to get login time: %v", err)
	}

	// Get desktop ready time via CGO to OSLogStore (Dock checkin with launchservicesd)
	if drt, err := GetDesktopReadyTime(); err == nil {
		result.DesktopReadyTime = drt
	} else {
		log.Warnf("logonduration: failed to get desktop ready time: %v", err)
	}

	return result
}
