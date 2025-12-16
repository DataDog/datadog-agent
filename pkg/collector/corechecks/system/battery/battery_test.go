// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (darwin || windows) && test

package battery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// setupMocks is a test helper that mocks hasBatteryAvailableFunc and getBatteryInfoFunc
// Returns a cleanup function that should be deferred
func setupMocks(hasBattery bool, info *batteryInfo) func() {
	originalHasBattery := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return hasBattery, nil
	}
	originalGetBatteryInfo := getBatteryInfoFunc
	getBatteryInfoFunc = func() (*batteryInfo, error) {
		return info, nil
	}
	return func() {
		hasBatteryAvailableFunc = originalHasBattery
		getBatteryInfoFunc = originalGetBatteryInfo
	}
}

// optFloat64 is a helper to create an option.Option[float64] with a value
func optFloat64(v float64) option.Option[float64] {
	return option.New(v)
}

// TestBatteryCheckWithMockedData tests the check with mocked battery info
func TestBatteryCheckWithMockedData(t *testing.T) {
	defer setupMocks(true, &batteryInfo{
		cycleCount:         optFloat64(150),
		designedCapacity:   optFloat64(100000),
		maximumCapacity:    optFloat64(95000),
		maximumCapacityPct: optFloat64(95.0),
		currentChargePct:   optFloat64(84.21),
		voltage:            optFloat64(12450),
		chargeRate:         optFloat64(-2500),
		powerState:         []string{"power_state:battery_discharging"},
	})()

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
	mockSender.AssertMetric(t, "Gauge", "system.battery.current_charge_pct", 84.21, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.voltage", 12450.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.charge_rate", -2500.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.power_state", 1.0, "", []string{"power_state:battery_discharging"})

	mockSender.AssertNumberOfCalls(t, "Gauge", 8)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestBatteryConfigure tests configuration
func TestBatteryConfigure(t *testing.T) {
	defer setupMocks(true, &batteryInfo{})()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)
}

// TestConfigureSkipsCheckWhenNoBattery tests that Configure returns ErrSkipCheckInstance when no battery
func TestConfigureSkipsCheckWhenNoBattery(t *testing.T) {
	// Mock hasBatteryAvailableFunc to return false (no battery)
	origFunc := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return false, nil
	}
	defer func() { hasBatteryAvailableFunc = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	// Should return ErrSkipCheckInstance
	require.Error(t, err)
	assert.Equal(t, check.ErrSkipCheckInstance, err)
}

// TestConfigureWithBatteryCheckError tests error handling from hasBatteryAvailableFunc
func TestConfigureWithBatteryCheckError(t *testing.T) {
	// Mock hasBatteryAvailableFunc to return an error
	origFunc := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return false, errors.New("failed to check battery")
	}
	defer func() { hasBatteryAvailableFunc = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	// Should return the error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check battery")
}

// TestBatteryMultipleRuns tests multiple consecutive runs
func TestBatteryMultipleRuns(t *testing.T) {
	originalHasBattery := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return true, nil
	}
	defer func() { hasBatteryAvailableFunc = originalHasBattery }()

	callCount := 0
	originalFunc := getBatteryInfoFunc
	getBatteryInfoFunc = func() (*batteryInfo, error) {
		callCount++
		return &batteryInfo{
			cycleCount:         option.New(150.0),
			designedCapacity:   option.New(100000.0),
			maximumCapacity:    option.New(95000.0),
			maximumCapacityPct: option.New(95.0),
			currentChargePct:   option.New(85.0 - float64(callCount*5.0)),
			voltage:            option.New(12300 - float64(callCount*50)),
			chargeRate:         option.New(-2000 - float64(callCount*100)),
			powerState:         []string{"power_state:battery_discharging"},
		}, nil
	}
	defer func() { getBatteryInfoFunc = originalFunc }()

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

	// getBatteryInfoFunc is called 3 times during Run (hasBatteryAvailableFunc is called during Configure)
	assert.Equal(t, 3, callCount)
	mockSender.AssertNumberOfCalls(t, "Commit", 3)
}

// TestBatteryHealthLevels tests different battery health scenarios
func TestBatteryHealthLevels(t *testing.T) {
	tests := []struct {
		name             string
		designedCapacity float64
		maximumCapacity  float64
		expectedHealth   float64
	}{
		{
			name:             "Good Battery",
			designedCapacity: 50000,
			maximumCapacity:  48000,
			expectedHealth:   96.0,
		},
		{
			name:             "Degraded Battery",
			designedCapacity: 50000,
			maximumCapacity:  35000,
			expectedHealth:   70.0,
		},
		{
			name:             "Very Degraded Battery",
			designedCapacity: 50000,
			maximumCapacity:  20000,
			expectedHealth:   40.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer setupMocks(true, &batteryInfo{
				cycleCount:         optFloat64(100),
				designedCapacity:   optFloat64(tt.designedCapacity),
				maximumCapacity:    optFloat64(tt.maximumCapacity),
				maximumCapacityPct: optFloat64(tt.expectedHealth),
				currentChargePct:   optFloat64(50.0),
				voltage:            optFloat64(12500),
				chargeRate:         optFloat64(0),
				powerState:         []string{"power_state:battery_power_on_line"},
			})()

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
				tt.maximumCapacity, "", []string(nil))
			mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity_pct",
				tt.expectedHealth, "", []string(nil))
		})
	}
}

// TestBatteryDischargeSimulation simulates battery discharge over time
func TestBatteryDischargeSimulation(t *testing.T) {
	originalHasBattery := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return true, nil
	}
	defer func() { hasBatteryAvailableFunc = originalHasBattery }()

	charges := []float64{100.0, 85.0, 70.0, 50.0, 25.0}
	currentIndex := 0

	originalFunc := getBatteryInfoFunc
	getBatteryInfoFunc = func() (*batteryInfo, error) {
		charge := charges[currentIndex]
		if currentIndex < len(charges)-1 {
			currentIndex++
		}
		return &batteryInfo{
			cycleCount:         option.New(150.0),
			designedCapacity:   option.New(50000.0),
			maximumCapacity:    option.New(48000.0),
			maximumCapacityPct: option.New(96.0),
			currentChargePct:   option.New(charge),
			voltage:            option.New(12500 - (charge * 5)),
			chargeRate:         option.New(-1500 - (charge * 2)),
			powerState:         []string{"power_state:battery_discharging"},
		}, nil
	}
	defer func() { getBatteryInfoFunc = originalFunc }()

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
			defer setupMocks(true, &batteryInfo{
				cycleCount:         optFloat64(100),
				designedCapacity:   optFloat64(50000),
				maximumCapacity:    optFloat64(48000),
				maximumCapacityPct: optFloat64(96.0),
				currentChargePct:   optFloat64(75.0),
				voltage:            optFloat64(12400),
				chargeRate:         optFloat64(-1800),
				powerState:         tt.powerState,
			})()

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
