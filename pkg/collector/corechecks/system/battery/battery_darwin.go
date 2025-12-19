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

import "github.com/DataDog/datadog-agent/pkg/util/option"

// optionalInt converts C.OptionalInt to option.Option[float64]
func optionalInt(o C.OptionalInt) option.Option[float64] {
	if !o.hasValue {
		return option.None[float64]()
	}
	return option.New(float64(o.value))
}

// optionalBool converts C.OptionalBool to option.Option[bool]
func optionalBool(o C.OptionalBool) option.Option[bool] {
	if !o.hasValue {
		return option.None[bool]()
	}
	return option.New(bool(o.value))
}

func hasBatteryAvailable() (bool, error) {
	cInfo := C.getBatteryInfo()
	return bool(cInfo.found), nil
}

// getBatteryInfo retrieves battery information from IOKit
func getBatteryInfo() (*batteryInfo, error) {
	cInfo := C.getBatteryInfo()
	return convertCBatteryInfo(cInfo), nil
}

// convertCBatteryInfo converts a C.BatteryInfo struct to a Go batteryInfo struct
func convertCBatteryInfo(cInfo C.BatteryInfo) *batteryInfo {
	info := &batteryInfo{
		powerState: getPowerStateTags(cInfo.isCharging, cInfo.externalConnected),
	}

	designCapacity := optionalInt(cInfo.designCapacity)
	appleRawMaxCapacity := optionalInt(cInfo.appleRawMaxCapacity)
	voltage := optionalInt(cInfo.voltage)

	info.cycleCount = optionalInt(cInfo.cycleCount)
	info.currentChargePct = optionalInt(cInfo.currentCapacity)
	info.voltage = voltage

	// Calculate derived metrics if we have the required values
	designCapVal, hasDesignCap := designCapacity.Get()
	appleRawMaxCapVal, hasAppleRawMaxCap := appleRawMaxCapacity.Get()
	voltageVal, hasVoltage := voltage.Get()

	if hasDesignCap && hasAppleRawMaxCap {
		maxCapPct := min(appleRawMaxCapVal/designCapVal*100.0, 100.0)
		info.maximumCapacityPct = option.New(maxCapPct)

		if hasVoltage {
			// mAh * mV / 1000 = mWh
			designedCap := designCapVal * voltageVal / 1000.0
			maxCap := appleRawMaxCapVal * voltageVal / 1000.0
			info.designedCapacity = option.New(designedCap)
			info.maximumCapacity = option.New(maxCap)

			instantAmperageOpt := optionalInt(cInfo.instantAmperage)
			if instantAmperage, ok := instantAmperageOpt.Get(); ok {
				chargeRate := instantAmperage * voltageVal / 1000.0
				info.chargeRate = option.New(chargeRate)
			}
		}
	}

	return info
}

func getPowerStateTags(isCharging, externalConnected C.OptionalBool) []string {
	powerStateTags := []string{}

	chargingOpt := optionalBool(isCharging)
	if charging, ok := chargingOpt.Get(); ok {
		if charging {
			powerStateTags = append(powerStateTags, "power_state:battery_charging")
		} else {
			powerStateTags = append(powerStateTags, "power_state:battery_discharging")
		}
	}

	connectedOpt := optionalBool(externalConnected)
	if connected, ok := connectedOpt.Get(); ok && connected {
		powerStateTags = append(powerStateTags, "power_state:battery_power_on_line")
	}

	return powerStateTags
}
