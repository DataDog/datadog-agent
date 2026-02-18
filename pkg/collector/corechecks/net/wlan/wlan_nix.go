// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"errors"
)

// getWiFiInfo is a package-level function variable for testability
// Tests can reassign this to mock WiFi data retrieval
var getWiFiInfo func() (wifiInfo, error)

// GetWiFiInfo retrieves WiFi information (not supported on this platform)
func (c *WLANCheck) GetWiFiInfo() (wifiInfo, error) {
	// Check for test override
	if getWiFiInfo != nil {
		return getWiFiInfo()
	}

	return wifiInfo{}, errors.New("wifi info only supported on macOS and Windows")
}
