// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package logonduration

import (
	"errors"
	"time"
)

// GetBootTime is not implemented on this platform
func GetBootTime() (time.Time, error) {
	return time.Time{}, errors.New("logonduration: not implemented on this platform")
}

// GetLoginWindowTime is not implemented on this platform
func GetLoginWindowTime(_ time.Time) (time.Time, error) {
	return time.Time{}, errors.New("logonduration: not implemented on this platform")
}

// GetLoginTime is not implemented on this platform
func GetLoginTime(_ time.Time) (time.Time, error) {
	return time.Time{}, errors.New("logonduration: not implemented on this platform")
}

// IsFileVaultEnabled is not implemented on this platform
func IsFileVaultEnabled() (bool, error) {
	return false, errors.New("logonduration: not implemented on this platform")
}

// GetLoginTimestamps is not implemented on this platform
func GetLoginTimestamps() *LoginTimestamps {
	return &LoginTimestamps{
		Error: "logonduration: not implemented on this platform",
	}
}

// GetDesktopReadyData is not implemented on this platform
func GetDesktopReadyData() (*DesktopReadyData, error) {
	return nil, errors.New("logonduration: not implemented on this platform")
}
