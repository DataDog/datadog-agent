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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// setupBatteryAvailable is a test helper that mocks HasBatteryAvailable to return true
// Returns a cleanup function that should be deferred
func setupBatteryAvailable() func() {
	original := HasBatteryAvailable
	HasBatteryAvailable = func() (bool, error) {
		return true, nil
	}
	return func() {
		HasBatteryAvailable = original
	}
}

// TestBatteryCheckWithMockedData tests the check with mocked battery info
func TestBatteryCheckWithMockedData(t *testing.T) {
	defer setupBatteryAvailable()()

	// Mock the battery query function
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95000,
			MaximumCapacityPct:  95.0,
			CycleCount:          150,
			CurrentCharge:       84.21,
			Voltage:             12450, // 12.45V in mV
			ChargeRate:          -2500, // -2.5A discharge rate in mA
			PowerState:          []string{"power_state:battery_discharging"},
			HasData:             true,
		}, nil
	}
	defer func() {
		QueryBatteryInfo = originalFunc
	}()

	// Create check
	batteryCheck := &Check{}

	// Setup mock sender
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// Run check
	err = batteryCheck.Run()
	require.NoError(t, err)

	// Assert metrics were submitted
	mockSender.AssertMetric(t, "Gauge", "system.battery.designed_capacity", 100000.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity", 95000.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity_pct", 95.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.cycle_count", 150.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.current_charge", 84.21, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.voltage", 12450.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.charge_rate", -2500.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.power_state", 1.0, "", []string{"power_state:battery_discharging"})

	mockSender.AssertNumberOfCalls(t, "Gauge", 8)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestBatteryConfigure tests configuration
func TestBatteryConfigure(t *testing.T) {
	defer setupBatteryAvailable()()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)
}

// TestConfigureSkipsCheckWhenNoBattery tests that Configure returns ErrSkipCheckInstance when no battery
func TestConfigureSkipsCheckWhenNoBattery(t *testing.T) {
	// Mock hasBatteryAvailable to return false (no battery)
	origFunc := HasBatteryAvailable
	HasBatteryAvailable = func() (bool, error) {
		return false, nil // No battery
	}
	defer func() { HasBatteryAvailable = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	// Should return ErrSkipCheckInstance
	require.Error(t, err)
	assert.Equal(t, check.ErrSkipCheckInstance, err)
}

// TestConfigureWithBatteryCheckError tests error handling from hasBatteryAvailable
func TestConfigureWithBatteryCheckError(t *testing.T) {
	// Mock hasBatteryAvailable to return an error
	origFunc := HasBatteryAvailable
	HasBatteryAvailable = func() (bool, error) {
		return false, errors.New("failed to check battery")
	}
	defer func() { HasBatteryAvailable = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	// Should return the error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check battery")
}

// TestBatteryMultipleRuns tests multiple consecutive runs
func TestBatteryMultipleRuns(t *testing.T) {
	defer setupBatteryAvailable()()

	callCount := 0
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		callCount++
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95000,
			MaximumCapacityPct:  95.0,
			CycleCount:          150,
			CurrentCharge:       85.0 - float64(callCount*5.0),  // Simulate discharge (percentage)
			Voltage:             12300 - float64(callCount*50),  // Voltage drops as battery discharges
			ChargeRate:          -2000 - float64(callCount*100), // Discharge rate increases
			PowerState:          []string{"power_state:battery_discharging"},
			HasData:             true,
		}, nil
	}
	defer func() {
		QueryBatteryInfo = originalFunc
	}()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// Run multiple times
	for i := 0; i < 3; i++ {
		err = batteryCheck.Run()
		require.NoError(t, err)
	}

	assert.Equal(t, 3, callCount)
	mockSender.AssertNumberOfCalls(t, "Commit", 3)
}

// TestBatteryHealthLevels tests different battery health scenarios
func TestBatteryHealthLevels(t *testing.T) {
	defer setupBatteryAvailable()()

	tests := []struct {
		name                string
		designedCapacity    float64
		fullChargedCapacity float64
		expectedHealth      float64
	}{
		{
			name:                "Good Battery",
			designedCapacity:    50000,
			fullChargedCapacity: 48000,
			expectedHealth:      96.0,
		},
		{
			name:                "Degraded Battery",
			designedCapacity:    50000,
			fullChargedCapacity: 35000,
			expectedHealth:      70.0,
		},
		{
			name:                "Very Degraded Battery",
			designedCapacity:    50000,
			fullChargedCapacity: 20000,
			expectedHealth:      40.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFunc := QueryBatteryInfo
			QueryBatteryInfo = func() (*BatteryInfo, error) {
				healthPercent := (tt.fullChargedCapacity / tt.designedCapacity) * 100
				return &BatteryInfo{
					DesignedCapacity:    tt.designedCapacity,
					FullChargedCapacity: tt.fullChargedCapacity,
					MaximumCapacityPct:  healthPercent,
					CycleCount:          100,
					CurrentCharge:       50.0,
					Voltage:             12500, // 12.5V
					ChargeRate:          0,     // Not charging/discharging
					PowerState:          []string{"power_state:battery_power_on_line"},
					HasData:             true,
				}, nil
			}
			defer func() {
				QueryBatteryInfo = originalFunc
			}()

			batteryCheck := &Check{}
			senderManager := mocksender.CreateDefaultDemultiplexer()
			err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
			require.NoError(t, err)

			mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
			mockSender.SetupAcceptAll()

			err = batteryCheck.Run()
			require.NoError(t, err)

			mockSender.AssertMetric(t, "Gauge", "system.battery.designed_capacity",
				tt.designedCapacity, "", []string(nil))
			mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity",
				tt.fullChargedCapacity, "", []string(nil))
			mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity_pct",
				tt.expectedHealth, "", []string(nil))
		})
	}
}

// TestBatteryDischargeSimulation simulates battery discharge over time
func TestBatteryDischargeSimulation(t *testing.T) {
	defer setupBatteryAvailable()()

	charges := []float64{100.0, 85.0, 70.0, 50.0, 25.0}
	currentIndex := 0

	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		charge := charges[currentIndex]
		if currentIndex < len(charges)-1 {
			currentIndex++
		}
		return &BatteryInfo{
			DesignedCapacity:    50000,
			FullChargedCapacity: 48000,
			MaximumCapacityPct:  96.0, // Percentage (48000/50000 * 100)
			CycleCount:          150,
			CurrentCharge:       charge,               // Already percentage
			Voltage:             12500 - (charge * 5), // Voltage decreases as charge decreases
			ChargeRate:          -1500 - (charge * 2), // Discharge rate varies with charge
			PowerState:          []string{"power_state:battery_discharging"},
			HasData:             true,
		}, nil
	}
	defer func() {
		QueryBatteryInfo = originalFunc
	}()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)

	mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// Simulate multiple runs (like real agent behavior)
	for i := range charges {
		err = batteryCheck.Run()
		require.NoError(t, err, "Run %d failed", i+1)
	}

	// Should have called Gauge 8 times per run (5 runs = 40 total)
	mockSender.AssertNumberOfCalls(t, "Gauge", 8*5)
	mockSender.AssertNumberOfCalls(t, "Commit", 5)
}

// TestBatteryPowerStates tests different power state scenarios
func TestBatteryPowerStates(t *testing.T) {
	defer setupBatteryAvailable()()

	tests := []struct {
		name          string
		powerState    []string
		expectedValue float64
		expectedTags  []string
	}{
		{
			name:          "On AC Power",
			powerState:    []string{"power_state:battery_power_on_line"},
			expectedValue: 1.0,
			expectedTags:  []string{"power_state:battery_power_on_line"},
		},
		{
			name:          "Discharging",
			powerState:    []string{"power_state:battery_discharging"},
			expectedValue: 1.0,
			expectedTags:  []string{"power_state:battery_discharging"},
		},
		{
			name:          "Charging",
			powerState:    []string{"power_state:battery_charging"},
			expectedValue: 1.0,
			expectedTags:  []string{"power_state:battery_charging"},
		},
		{
			name:          "Critical",
			powerState:    []string{"power_state:battery_critical"},
			expectedValue: 1.0,
			expectedTags:  []string{"power_state:battery_critical"},
		},
		{
			name:          "Multiple States",
			powerState:    []string{"power_state:battery_power_on_line", "power_state:battery_charging"},
			expectedValue: 1.0,
			expectedTags:  []string{"power_state:battery_power_on_line", "power_state:battery_charging"},
		},
		{
			name:          "Unknown State",
			powerState:    []string{},
			expectedValue: 0.0,
			expectedTags:  []string{"power_state:unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFunc := QueryBatteryInfo
			QueryBatteryInfo = func() (*BatteryInfo, error) {
				return &BatteryInfo{
					DesignedCapacity:    50000,
					FullChargedCapacity: 48000,
					MaximumCapacityPct:  96.0,
					CycleCount:          100,
					CurrentCharge:       75.0,
					Voltage:             12400, // 12.4V
					ChargeRate:          -1800, // Discharging at 1.8A
					PowerState:          tt.powerState,
					HasData:             true,
				}, nil
			}
			defer func() {
				QueryBatteryInfo = originalFunc
			}()

			batteryCheck := &Check{}
			senderManager := mocksender.CreateDefaultDemultiplexer()
			err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
			require.NoError(t, err)

			mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
			mockSender.SetupAcceptAll()

			err = batteryCheck.Run()
			require.NoError(t, err)

			// Assert power state metric
			mockSender.AssertMetric(t, "Gauge", "system.battery.power_state",
				tt.expectedValue, "", tt.expectedTags)
		})
	}
}
