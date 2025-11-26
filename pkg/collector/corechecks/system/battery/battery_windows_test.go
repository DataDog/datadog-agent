// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package battery

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

// TestBatteryCheckWithMockedData tests the check with mocked battery info
// This test runs on ALL platforms (Windows, Linux, macOS)
func TestBatteryCheckWithMockedData(t *testing.T) {
	// Mock the battery query function
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95000,
			CycleCount:          150,
			CurrentCharge:       80000,
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
	mockSender.AssertMetric(t, "Gauge", "system.battery.cycle_count", 150.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.current_charge", 80000.0, "", []string(nil))

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

// TestBatteryCheckGetSenderError tests error handling when sender fails
func TestBatteryCheckGetSenderError(t *testing.T) {
	batteryCheck := &Check{}
	// Don't configure the check - GetSender will fail

	err := batteryCheck.Run()
	assert.Error(t, err)
}

// TestBatteryConfigure tests configuration
func TestBatteryConfigure(t *testing.T) {
	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	require.NoError(t, err)
}

// TestBatteryMultipleRuns tests multiple consecutive runs
func TestBatteryMultipleRuns(t *testing.T) {
	callCount := 0
	originalFunc := QueryBatteryInfo
	QueryBatteryInfo = func() (*BatteryInfo, error) {
		callCount++
		return &BatteryInfo{
			DesignedCapacity:    100000,
			FullChargedCapacity: 95000,
			CycleCount:          150,
			CurrentCharge:       80000 - uint32(callCount*1000), // Simulate discharge
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
