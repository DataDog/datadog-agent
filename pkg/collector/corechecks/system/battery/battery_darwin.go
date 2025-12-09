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
import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// batteryInfo holds battery information from IOKit
type batteryInfo struct {
	found               bool
	cycleCount          float64
	designCapacity      float64 // mAh
	appleRawMaxCapacity float64 // mAh
	currentCapacity     float64 // % of design capacity
	voltage             float64 // mV
	instantAmperage     float64 // mA (negative = discharging, positive = charging)
	isCharging          bool
	externalConnected   bool
}

// Configure handles initial configuration/initialization of the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(senderManager, initConfig, data, source); err != nil {
		return err
	}

	info := getBatteryInfoFunc()
	if !info.found {
		log.Debugf("No battery found, skipping check")
		return check.ErrSkipCheckInstance
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	info := getBatteryInfoFunc()

	sender.Gauge("system.battery.cycle_count", info.cycleCount, "", nil)
	sender.Gauge("system.battery.designed_capacity", info.designCapacityMWh(), "", nil)
	sender.Gauge("system.battery.maximum_capacity", info.maxCapacityMWh(), "", nil)
	sender.Gauge("system.battery.maximum_capacity_pct", info.maxCapacityPercent(), "", nil)
	sender.Gauge("system.battery.current_charge", info.currentCapacity, "", nil)
	sender.Gauge("system.battery.voltage", info.voltage, "", nil)
	sender.Gauge("system.battery.charge_rate", info.chargeRateWatts(), "", nil)
	sender.Gauge("system.battery.power_state", 1.0, "", info.getPowerStateTags())

	return nil
}

// getBatteryInfo retrieves battery information from IOKit
func getBatteryInfo() batteryInfo {
	cInfo := C.getBatteryInfo()

	return batteryInfo{
		found:               bool(cInfo.found),
		cycleCount:          float64(cInfo.cycleCount),
		designCapacity:      float64(cInfo.designCapacity),
		appleRawMaxCapacity: float64(cInfo.appleRawMaxCapacity),
		currentCapacity:     float64(cInfo.currentCapacity),
		voltage:             float64(cInfo.voltage),
		instantAmperage:     float64(cInfo.instantAmperage),
		isCharging:          bool(cInfo.isCharging),
		externalConnected:   bool(cInfo.externalConnected),
	}
}

// getBatteryInfoFunc is a mockable function variable for retrieving battery information
var getBatteryInfoFunc = getBatteryInfo

// maxCapacityPercent calculates battery health as appleRawMaxCapacity / designCapacity
func (b batteryInfo) maxCapacityPercent() float64 {
	return min(b.appleRawMaxCapacity/b.designCapacity*100.0, 100.0)
}

// designCapacityMWh converts design capacity from mAh to mWh
func (b batteryInfo) designCapacityMWh() float64 {
	return b.designCapacity * b.voltage / 1000.0
}

// maxCapacityMWh converts max capacity from mAh to mWh
func (b batteryInfo) maxCapacityMWh() float64 {
	return b.appleRawMaxCapacity * b.voltage / 1000.0
}

// chargeRateWatts calculates the charge/discharge rate in watts
// Positive = charging, negative = discharging
func (b batteryInfo) chargeRateWatts() float64 {
	return b.instantAmperage * b.voltage / 1000.0
}

func (b batteryInfo) getPowerStateTags() []string {
	powerStateTags := []string{}
	if b.isCharging {
		powerStateTags = append(powerStateTags, "power_state:battery_charging")
	} else {
		powerStateTags = append(powerStateTags, "power_state:battery_discharging")
	}
	if b.externalConnected {
		powerStateTags = append(powerStateTags, "power_state:battery_power_on_line")
	}
	return powerStateTags
}
