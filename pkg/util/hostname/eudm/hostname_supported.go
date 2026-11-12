// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin || windows

package eudm

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/inventory/systeminfo"
)

var collectSystemInfo = systeminfo.Collect

// Get returns the device name, optionally suffixed with its device serial number.
// Missing identity is an error so callers can retain legacy hostname resolution.
func Get(useSerial bool) (string, error) {
	var serial string
	// System information is only needed for the optional serial suffix.
	if useSerial {
		info, err := collectSystemInfo()
		if err != nil {
			return "", fmt.Errorf("collecting device identity: %w", err)
		}
		if info == nil {
			return "", errors.New("device identity is unavailable")
		}
		serial = info.SerialNumber
	}
	name, err := deviceName()
	if err != nil {
		return "", err
	}
	return formatHostname(name, serial, useSerial)
}
