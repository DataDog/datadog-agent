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

// Get returns the normalized device name suffixed with its hardware serial.
// Missing identity is an error so callers can retain legacy hostname resolution.
func Get() (string, error) {
	info, err := collectSystemInfo()
	if err != nil {
		return "", fmt.Errorf("collecting device identity: %w", err)
	}
	if info == nil {
		return "", errors.New("device identity is unavailable")
	}
	name, err := deviceName(info)
	if err != nil {
		return "", err
	}
	return formatHostname(name, info.SerialNumber)
}
