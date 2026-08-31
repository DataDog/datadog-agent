// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package battery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestGetBatteryInfoMultipleBatteries(t *testing.T) {
	descriptors := []batteryDeviceDescriptor{
		{devicePath: "battery-0", instanceID: `ROOT\BATTERY\0000`},
		{devicePath: "battery-1", instanceID: `ROOT\BATTERY\0001`},
	}
	batteries := map[string]*windowsBattery{
		"battery-0": {
			descriptor: descriptors[0],
			info: BATTERY_INFORMATION{
				Capabilities:        BATTERY_SYSTEM_BATTERY,
				DesignedCapacity:    6000,
				FullChargedCapacity: 5400,
				CycleCount:          101,
			},
			status:     BATTERY_STATUS{PowerState: BATTERY_DISCHARGING, Capacity: 4050, Voltage: 12000, Rate: -900},
			serial:     "SERIAL-1",
			deviceName: "SimBatt One",
		},
		"battery-1": {
			descriptor: descriptors[1],
			info: BATTERY_INFORMATION{
				Capabilities:        BATTERY_SYSTEM_BATTERY,
				DesignedCapacity:    4000,
				FullChargedCapacity: 3600,
				CycleCount:          202,
			},
			status:     BATTERY_STATUS{PowerState: BATTERY_POWER_ON_LINE, Capacity: 1800, Voltage: 11500, Rate: -450},
			serial:     "SERIAL-2",
			deviceName: "SimBatt Two",
		},
	}

	restoreWindowsBatteryMocks(t, descriptors, func(descriptor batteryDeviceDescriptor) (*windowsBattery, error) {
		return batteries[descriptor.devicePath], nil
	})

	infos, err := getBatteryInfo()
	require.NoError(t, err)
	require.Len(t, infos, 3)

	assert.Equal(t, []string{"battery_slot:root_battery_0000", "battery_serial:serial-1", "battery_device_name:simbatt_one"}, infos[0].tags)
	assertOptionValue(t, infos[0].currentChargePct, 75)
	assertOptionValue(t, infos[0].cycleCount, 101)
	assertOptionValue(t, infos[0].voltage, 12000)

	assert.Equal(t, []string{"battery_slot:total"}, infos[2].tags)
	assertOptionValue(t, infos[2].designedCapacity, 10000)
	assertOptionValue(t, infos[2].maximumCapacity, 9000)
	assertOptionValue(t, infos[2].maximumCapacityPct, 90)
	assertOptionValue(t, infos[2].currentChargePct, 65)
	assertOptionValue(t, infos[2].chargeRate, -1350)
	assert.Equal(t, []string{"power_state:battery_power_on_line", "power_state:battery_discharging"}, infos[2].powerState)
	_, hasCycleCount := infos[2].cycleCount.Get()
	_, hasVoltage := infos[2].voltage.Get()
	assert.False(t, hasCycleCount)
	assert.False(t, hasVoltage)
}

func TestGetBatteryInfoEmitsTotalForOneBattery(t *testing.T) {
	descriptor := batteryDeviceDescriptor{devicePath: "battery-0", instanceID: `ROOT\BATTERY\0000`}
	battery := &windowsBattery{
		descriptor: descriptor,
		info: BATTERY_INFORMATION{
			Capabilities:        BATTERY_SYSTEM_BATTERY,
			DesignedCapacity:    6000,
			FullChargedCapacity: 5400,
		},
		status: BATTERY_STATUS{Capacity: 4050, Voltage: 12000, Rate: -900},
	}
	restoreWindowsBatteryMocks(t, []batteryDeviceDescriptor{descriptor}, func(batteryDeviceDescriptor) (*windowsBattery, error) {
		return battery, nil
	})

	infos, err := getBatteryInfo()
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, []string{"battery_slot:total"}, infos[1].tags)
}

func TestGetBatteryInfoSuppressesPartialTotal(t *testing.T) {
	descriptors := []batteryDeviceDescriptor{
		{devicePath: "battery-0", instanceID: `ROOT\BATTERY\0000`},
		{devicePath: "battery-1", instanceID: `ROOT\BATTERY\0001`},
	}
	battery := &windowsBattery{
		descriptor: descriptors[1],
		info: BATTERY_INFORMATION{
			Capabilities:        BATTERY_SYSTEM_BATTERY,
			DesignedCapacity:    6000,
			FullChargedCapacity: 5400,
		},
		status: BATTERY_STATUS{Capacity: 4050, Voltage: 12000, Rate: -900},
	}
	restoreWindowsBatteryMocks(t, descriptors, func(descriptor batteryDeviceDescriptor) (*windowsBattery, error) {
		if descriptor.devicePath == "battery-0" {
			return nil, errors.New("query failed")
		}
		return battery, nil
	})

	infos, err := getBatteryInfo()
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "battery_slot:root_battery_0001", infos[0].tags[0])
}

func TestBatterySlotValueUsesUINumber(t *testing.T) {
	descriptor := batteryDeviceDescriptor{
		devicePath:  "battery-a",
		instanceID:  `ACPI\BATTERY\0000`,
		uiNumber:    30,
		hasUINumber: true,
	}

	assert.Equal(t, "30", batterySlotValue(descriptor))
}

func TestBatterySlotValueFallsBackToInstanceID(t *testing.T) {
	descriptor := batteryDeviceDescriptor{
		devicePath: "battery-root-0",
		instanceID: `ROOT\BATTERY\0000`,
	}

	assert.Equal(t, `ROOT\BATTERY\0000`, batterySlotValue(descriptor))
}

func TestBatterySlotValueFallsBackToDevicePath(t *testing.T) {
	descriptor := batteryDeviceDescriptor{devicePath: `\\?\battery#device`}

	assert.Equal(t, `\\?\battery#device`, batterySlotValue(descriptor))
}

func restoreWindowsBatteryMocks(t *testing.T, descriptors []batteryDeviceDescriptor, query func(batteryDeviceDescriptor) (*windowsBattery, error)) {
	originalEnumerate := enumerateBatteryDeviceDescriptorsFunc
	originalQuery := queryBatteryDeviceFunc
	enumerateBatteryDeviceDescriptorsFunc = func() ([]batteryDeviceDescriptor, error) {
		return descriptors, nil
	}
	queryBatteryDeviceFunc = query
	t.Cleanup(func() {
		enumerateBatteryDeviceDescriptorsFunc = originalEnumerate
		queryBatteryDeviceFunc = originalQuery
	})
}

func assertOptionValue(t *testing.T, value option.Option[float64], expected float64) {
	actual, ok := value.Get()
	require.True(t, ok)
	assert.Equal(t, expected, actual)
}
