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

func hasBatteryAvailable() (bool, error) {
	cInfo := C.getBatteryInfo()
	return bool(cInfo.found), nil
}

// getBatteryInfo retrieves battery information from IOKit
func getBatteryInfo() (batteryInfo, error) {
	cInfo := C.getBatteryInfo()

	designCapacity := float64(cInfo.designCapacity)           // mAh
	appleRawMaxCapacity := float64(cInfo.appleRawMaxCapacity) // mAh
	voltage := float64(cInfo.voltage)                         // mV
	instantAmperage := float64(cInfo.instantAmperage)         // mA

	return batteryInfo{
		cycleCount:         float64(cInfo.cycleCount),
		designedCapacity:   designCapacity * voltage / 1000.0,                    // mWh
		maximumCapacity:    appleRawMaxCapacity * voltage / 1000.0,               // mWh
		maximumCapacityPct: min(appleRawMaxCapacity/designCapacity*100.0, 100.0), // percentage capped at 100
		currentCharge:      float64(cInfo.currentCapacity),                       // percentage
		voltage:            voltage,                                              // mV
		chargeRate:         instantAmperage * voltage / 1000.0,                   // mW
		powerState:         getPowerStateTags(bool(cInfo.isCharging), bool(cInfo.externalConnected)),
	}, nil
}

func getPowerStateTags(isCharging, externalConnected bool) []string {
	powerStateTags := []string{}
	if isCharging {
		powerStateTags = append(powerStateTags, "power_state:battery_charging")
	} else {
		powerStateTags = append(powerStateTags, "power_state:battery_discharging")
	}
	if externalConnected {
		powerStateTags = append(powerStateTags, "power_state:battery_power_on_line")
	}
	return powerStateTags
}
