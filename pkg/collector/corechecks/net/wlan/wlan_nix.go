// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import "fmt"

func GetWiFiInfo() (wifiInfo, error) {
	return wifiInfo{}, fmt.Errorf("wifi info only supported on macOS and Windows")
}
