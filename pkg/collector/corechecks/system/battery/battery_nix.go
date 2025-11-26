// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package battery

import (
	"fmt"
)

func queryBatteryInfo() (*BatteryInfo, error) {
	return nil, fmt.Errorf("battery check is currently only supported on Windows")
}
