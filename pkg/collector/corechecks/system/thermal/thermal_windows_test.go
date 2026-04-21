// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package thermal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pdhtest "github.com/DataDog/datadog-agent/pkg/util/pdhutil"
)

// toCelsius converts Kelvin to Celsius using runtime float64 arithmetic,
// matching the production code behavior (avoids compile-time constant folding).
func toCelsius(kelvin float64) float64 {
	return kelvin - kelvinOffset
}

func TestThermalCheck(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.AddCounterInstance("Thermal Zone Information", "tz.thm0")

	// High Precision Temperature (3452 tenths-of-K = 345.2 K = 72.05 C) takes precedence
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\Temperature", 345.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\High Precision Temperature", 3452.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\% Passive Limit", 100.0)

	check := new(thermalCheck)
	mock := mocksender.NewMockSender(check.ID())
	check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	mock.On("Gauge", "system.thermal.temperature", toCelsius(3452.0/10.0), "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Gauge", "system.thermal.passive_limit", 100.0, "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	check.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestThermalCheckMultipleZones(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.AddCounterInstance("Thermal Zone Information", "tz.thm0")
	pdhtest.AddCounterInstance("Thermal Zone Information", "tz.thm1")

	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\Temperature", 345.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\High Precision Temperature", 3450.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\% Passive Limit", 100.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm1)\\Temperature", 360.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm1)\\High Precision Temperature", 3600.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm1)\\% Passive Limit", 80.0)

	check := new(thermalCheck)
	mock := mocksender.NewMockSender(check.ID())
	check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	// Zone 0: HP = 3450 tenths-K = 345.0 K = 71.85°C, not throttled
	mock.On("Gauge", "system.thermal.temperature", toCelsius(3450.0/10.0), "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Gauge", "system.thermal.passive_limit", 100.0, "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)

	// Zone 1: HP = 3600 tenths-K = 360.0 K = 86.85°C, throttled to 80%
	mock.On("Gauge", "system.thermal.temperature", toCelsius(3600.0/10.0), "", []string{"thermal_zone:tz.thm1"}).Return().Times(1)
	mock.On("Gauge", "system.thermal.passive_limit", 80.0, "", []string{"thermal_zone:tz.thm1"}).Return().Times(1)

	mock.On("Commit").Return().Times(1)
	check.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 4)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestThermalCheckThrottled(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.AddCounterInstance("Thermal Zone Information", "tz.thm0")

	// Precision of 0.1 K preserved via High Precision Temperature
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\Temperature", 380.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\High Precision Temperature", 3807.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\% Passive Limit", 50.0)

	check := new(thermalCheck)
	mock := mocksender.NewMockSender(check.ID())
	check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	// HP = 3807 tenths-K = 380.7 K = 107.55°C, throttled to 50%
	mock.On("Gauge", "system.thermal.temperature", toCelsius(3807.0/10.0), "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Gauge", "system.thermal.passive_limit", 50.0, "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	check.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

// TestThermalCheckFallbackToTemperature verifies that when High Precision
// Temperature is unavailable for an instance, the check falls back to the
// lower-precision Temperature counter.
func TestThermalCheckFallbackToTemperature(t *testing.T) {
	pdhtest.SetupTesting("..\\testfiles\\counter_indexes_en-us.txt", "..\\testfiles\\allcounters_en-us.txt")
	pdhtest.AddCounterInstance("Thermal Zone Information", "tz.thm0")

	// Only Temperature is available, not High Precision
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\Temperature", 345.0)
	pdhtest.SetQueryReturnValue("\\\\.\\Thermal Zone Information(tz.thm0)\\% Passive Limit", 100.0)

	check := new(thermalCheck)
	mock := mocksender.NewMockSender(check.ID())
	check.Configure(mock.GetSenderManager(), integration.FakeConfigHash, nil, nil, "test", "provider")

	// Falls back to Temperature: 345 K = 71.85°C
	mock.On("Gauge", "system.thermal.temperature", toCelsius(345.0), "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Gauge", "system.thermal.passive_limit", 100.0, "", []string{"thermal_zone:tz.thm0"}).Return().Times(1)
	mock.On("Commit").Return().Times(1)
	check.Run()

	mock.AssertExpectations(t)
	mock.AssertNumberOfCalls(t, "Gauge", 2)
	mock.AssertNumberOfCalls(t, "Commit", 1)
}

func TestIsNotTotal(t *testing.T) {
	assert.True(t, isNotTotal("tz.thm0"))
	assert.True(t, isNotTotal("tz.thm1"))
	assert.False(t, isNotTotal("_Total"))
}
