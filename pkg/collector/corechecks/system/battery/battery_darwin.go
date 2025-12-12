// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package battery

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework IOKit

#include "battery_darwin.h"
*/
import "C"

// unwrapInt converts C.OptionalInt to *float64 (nil if not present)
func unwrapInt(o C.OptionalInt) *float64 {
	if !o.hasValue {
		return nil
	}
	v := float64(o.value)
	return &v
}

// unwrapBool converts C.OptionalBool to *bool (nil if not present)
func unwrapBool(o C.OptionalBool) *bool {
	if !o.hasValue {
		return nil
	}
	v := bool(o.value)
	return &v
}

func hasBatteryAvailable() (bool, error) {
	cInfo := C.getBatteryInfo()
	return bool(cInfo.found), nil
}

// getBatteryInfo retrieves battery information from IOKit
func getBatteryInfo() (*batteryInfo, error) {
	cInfo := C.getBatteryInfo()

	info := &batteryInfo{
		powerState: getPowerStateTags(cInfo.isCharging, cInfo.externalConnected),
	}

	designCapacity := unwrapInt(cInfo.designCapacity)
	appleRawMaxCapacity := unwrapInt(cInfo.appleRawMaxCapacity)
	voltage := unwrapInt(cInfo.voltage)

	info.cycleCount = unwrapInt(cInfo.cycleCount)
	info.currentChargePct = unwrapInt(cInfo.currentCapacity)
	info.voltage = voltage

	// Calculate derived metrics if we have the required values
	if designCapacity != nil && appleRawMaxCapacity != nil {
		maxCapPct := min(*appleRawMaxCapacity / *designCapacity * 100.0, 100.0)
		info.maximumCapacityPct = &maxCapPct

		if voltage != nil {
			// mAh * mV / 1000 = mWh
			designedCap := *designCapacity * *voltage / 1000.0
			maxCap := *appleRawMaxCapacity * *voltage / 1000.0
			info.designedCapacity = &designedCap
			info.maximumCapacity = &maxCap

			if instantAmperage := unwrapInt(cInfo.instantAmperage); instantAmperage != nil {
				chargeRate := *instantAmperage * *voltage / 1000.0
				info.chargeRate = &chargeRate
			}
		}
	}

	return info, nil
}

func getPowerStateTags(isCharging, externalConnected C.OptionalBool) []string {
	powerStateTags := []string{}

	if charging := unwrapBool(isCharging); charging != nil {
		if *charging {
			powerStateTags = append(powerStateTags, "power_state:battery_charging")
		} else {
			powerStateTags = append(powerStateTags, "power_state:battery_discharging")
		}
	}

	if connected := unwrapBool(externalConnected); connected != nil && *connected {
		powerStateTags = append(powerStateTags, "power_state:battery_power_on_line")
	}

	return powerStateTags
}
