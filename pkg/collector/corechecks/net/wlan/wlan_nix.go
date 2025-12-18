// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"errors"

	"github.com/xeipuuv/gojsonschema"
)

// GetWiFiInfo retrieves WiFi information (not supported on this platform)
func (c *WLANCheck) GetWiFiInfo() (wifiInfo, error) {
	return wifiInfo{}, errors.New("wifi info only supported on macOS and Windows")
}

// createIPCResponseSchema is a stub for non-darwin platforms
func createIPCResponseSchema() (*gojsonschema.Schema, error) {
	return nil, errors.New("IPC schema validation only needed on macOS")
}
