// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin && test

package battery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertCBatteryInfo(t *testing.T) {
	info := convertCBatteryInfo(testCBatteryInfo)

	cycleCount, ok := info.cycleCount.Get()
	assert.True(t, ok)
	assert.Equal(t, 500.0, cycleCount)

	maxCapPct, ok := info.maximumCapacityPct.Get()
	assert.True(t, ok)
	assert.Equal(t, 90.0, maxCapPct) // 4500/5000 * 100

	designedCap, ok := info.designedCapacity.Get()
	assert.True(t, ok)
	assert.Equal(t, 60000.0, designedCap) // 5000 * 12000 / 1000

	maxCap, ok := info.maximumCapacity.Get()
	assert.True(t, ok)
	assert.Equal(t, 54000.0, maxCap) // 4500 * 12000 / 1000

	currentCharge, ok := info.currentChargePct.Get()
	assert.True(t, ok)
	assert.Equal(t, 80.0, currentCharge)

	voltage, ok := info.voltage.Get()
	assert.True(t, ok)
	assert.Equal(t, 12000.0, voltage)

	chargeRate, ok := info.chargeRate.Get()
	assert.True(t, ok)
	assert.Equal(t, -12000.0, chargeRate) // -1000 * 12000 / 1000

	assert.Contains(t, info.powerState, "power_state:battery_discharging")
}

func TestConvertCBatteryInfo_MissingVoltage(t *testing.T) {
	cInfo := testCBatteryInfo
	cInfo.voltage.hasValue = false

	info := convertCBatteryInfo(cInfo)

	// These require voltage to calculate - should not have values
	_, hasVoltage := info.voltage.Get()
	assert.False(t, hasVoltage)
	_, hasDesignedCap := info.designedCapacity.Get()
	assert.False(t, hasDesignedCap)
	_, hasMaxCap := info.maximumCapacity.Get()
	assert.False(t, hasMaxCap)
	_, hasChargeRate := info.chargeRate.Get()
	assert.False(t, hasChargeRate)

	// These don't need voltage - should have values
	_, hasCycleCount := info.cycleCount.Get()
	assert.True(t, hasCycleCount)
	_, hasMaxCapPct := info.maximumCapacityPct.Get()
	assert.True(t, hasMaxCapPct)
}
