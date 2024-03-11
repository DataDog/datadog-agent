// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package process converts Cisco SD-WAN api responses
package process

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	"math"
	"time"
)

// ProcessDeviceUptimes convert uptime dates to number of hundredths of a second since the device is up
func ProcessDeviceUptimes(devices []client.Device) map[string]float64 {
	uptimes := make(map[string]float64)
	now := time.Now().UnixMilli()

	for _, device := range devices {
		uptimes[device.SystemIP] = math.Round((float64(now) - float64(device.UptimeDate)) / 10) // In hundredths of a second, to match SNMP
	}

	return uptimes
}
