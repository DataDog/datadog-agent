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

// TestBatteryCheckWithMockedData tests the check with mocked battery info
// This test runs on ALL platforms (Windows, Linux, macOS)
func TestBatteryCheckWithMockedData(t *testing.T) {
	// Mock the battery query function
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95.0,
			CycleCount:          150,
			CurrentCharge:       84.21,
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
	mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity", 95.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.cycle_count", 150.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.current_charge", 84.21, "", []string(nil))

	mockSender.AssertNumberOfCalls(t, "Gauge", 4)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestBatteryCheckNoBattery tests behavior when no battery is found
func TestBatteryCheckNoBattery(t *testing.T) {
	originalFunc := queryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		return nil, errors.New("no battery info found")
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

	// Run should return an error
	err = batteryCheck.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no battery info found")

	// No metrics should be submitted
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 0)
}

// TestBatteryConfigure tests configuration
func TestBatteryConfigure(t *testing.T) {
	// Save and mock hasBatteryAvailable to return true
	origFunc := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return true, nil // Simulate battery present
	}
	defer func() { hasBatteryAvailableFunc = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)
}

// TestConfigureSkipsCheckWhenNoBattery tests that Configure returns ErrSkipCheckInstance when no battery
func TestConfigureSkipsCheckWhenNoBattery(t *testing.T) {
	// Mock hasBatteryAvailable to return false (no battery)
	origFunc := hasBatteryAvailableFunc
	hasBatteryAvailableFunc = func() (bool, error) {
		return false, nil // No battery
	}
	defer func() { hasBatteryAvailableFunc = origFunc }()

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
	callCount := 0
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		callCount++
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95.0, // Percentage
			CycleCount:          150,
			CurrentCharge:       85.0 - float64(callCount*5.0), // Simulate discharge (percentage)
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
					FullChargedCapacity: healthPercent,
					CycleCount:          100,
					CurrentCharge:       50.0,
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
				tt.expectedHealth, "", []string(nil))
		})
	}
}

// TestBatteryDischargeSimulation simulates battery discharge over time
func TestBatteryDischargeSimulation(t *testing.T) {
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
			FullChargedCapacity: 96.0, // Percentage (48000/50000 * 100)
			CycleCount:          150,
			CurrentCharge:       charge, // Already percentage
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

	// Should have called Gauge 4 times per run (5 runs = 20 total)
	mockSender.AssertNumberOfCalls(t, "Gauge", 4*5)
	mockSender.AssertNumberOfCalls(t, "Commit", 5)
}

// TestBatteryErrorScenarios tests various error conditions
func TestBatteryErrorScenarios(t *testing.T) {
	tests := []struct {
		name          string
		mockFunc      func() (*BatteryInfo, error)
		expectedError string
		expectMetrics bool
	}{
		{
			name: "Device access denied",
			mockFunc: func() (*BatteryInfo, error) {
				return nil, errors.New("Error creating file handle: access denied")
			},
			expectedError: "access denied",
			expectMetrics: false,
		},
		{
			name: "Invalid battery tag",
			mockFunc: func() (*BatteryInfo, error) {
				return nil, errors.New("Error querying battery tag")
			},
			expectedError: "battery tag",
			expectMetrics: false,
		},
		{
			name: "Battery information query failed",
			mockFunc: func() (*BatteryInfo, error) {
				return nil, errors.New("Error querying battery information")
			},
			expectedError: "battery information",
			expectMetrics: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFunc := QueryBatteryInfo
			QueryBatteryInfo = tt.mockFunc
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
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}

			if !tt.expectMetrics {
				mockSender.AssertNumberOfCalls(t, "Gauge", 0)
				mockSender.AssertNumberOfCalls(t, "Commit", 0)
			}
		})
	}
}
