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

	assert.Equal(t, 500.0, *info.cycleCount)
	assert.Equal(t, 90.0, *info.maximumCapacityPct)  // 4500/5000 * 100
	assert.Equal(t, 60000.0, *info.designedCapacity) // 5000 * 12000 / 1000
	assert.Equal(t, 54000.0, *info.maximumCapacity)  // 4500 * 12000 / 1000
	assert.Equal(t, 80.0, *info.currentChargePct)
	assert.Equal(t, 12000.0, *info.voltage)
	assert.Equal(t, -12000.0, *info.chargeRate) // -1000 * 12000 / 1000
	assert.Contains(t, info.powerState, "power_state:battery_discharging")
}

func TestConvertCBatteryInfo_MissingVoltage(t *testing.T) {
	cInfo := testCBatteryInfo
	cInfo.voltage.hasValue = false

	info := convertCBatteryInfo(cInfo)

	// These require voltage to calculate
	assert.Nil(t, info.voltage)
	assert.Nil(t, info.designedCapacity)
	assert.Nil(t, info.maximumCapacity)
	assert.Nil(t, info.chargeRate)

	// These don't need voltage
	assert.NotNil(t, info.cycleCount)
	assert.NotNil(t, info.maximumCapacityPct)
}
