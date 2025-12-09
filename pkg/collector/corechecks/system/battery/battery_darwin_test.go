// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin && test

package battery

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// setupBatteryAvailable is a test helper that mocks getBatteryInfoFunc to return a battery found
// Returns a cleanup function that should be deferred
func setupBatteryAvailable() func() {
	original := getBatteryInfoFunc
	getBatteryInfoFunc = func() batteryInfo {
		return batteryInfo{
			found:               true,
			cycleCount:          100,
			designCapacity:      8000,
			appleRawMaxCapacity: 7600,
			currentCapacity:     85,
			voltage:             12400,
			instantAmperage:     -1500,
			isCharging:          false,
			externalConnected:   false,
		}
	}
	return func() {
		getBatteryInfoFunc = original
	}
}

// TestBatteryCheckWithMockedData tests the check with mocked battery info
func TestBatteryCheckWithMockedData(t *testing.T) {
	defer setupBatteryAvailable()()

	// Mock the battery query function
	originalFunc := getBatteryInfoFunc
	getBatteryInfoFunc = func() batteryInfo {
		return batteryInfo{
			found:               true,
			cycleCount:          150,
			designCapacity:      8000,  // mAh
			appleRawMaxCapacity: 7600,  // mAh
			currentCapacity:     84,    // % of design capacity
			voltage:             12450, // mV
			instantAmperage:     -2500, // mA (negative = discharging)
			isCharging:          false,
			externalConnected:   false,
		}
	}
	defer func() {
		getBatteryInfoFunc = originalFunc
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

	// Calculate expected values
	// designCapacityMWh = 8000 * 12450 / 1000 = 99600 mWh
	// maxCapacityMWh = 7600 * 12450 / 1000 = 94620 mWh
	// maxCapacityPercent = 7600 / 8000 * 100 = 95%
	// chargeRateWatts = -2500 * 12450 / 1000 = -31125 mW (negative = discharging)

	// Assert metrics were submitted
	mockSender.AssertMetric(t, "Gauge", "system.battery.cycle_count", 150.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.designed_capacity", 99600.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity", 94620.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.maximum_capacity_pct", 95.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.current_charge", 84.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.voltage", 12450.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.charge_rate", -31125.0, "", []string(nil))
	mockSender.AssertMetric(t, "Gauge", "system.battery.power_state", 1.0, "", []string{"power_state:battery_discharging"})

	mockSender.AssertNumberOfCalls(t, "Gauge", 8)
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
	// Mock getBatteryInfoFunc to return no battery found
	origFunc := getBatteryInfoFunc
	getBatteryInfoFunc = func() batteryInfo {
		return batteryInfo{found: false}
	}
	defer func() { getBatteryInfoFunc = origFunc }()

	batteryCheck := &Check{}
	senderManager := mocksender.CreateDefaultDemultiplexer()

	err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	// Should return ErrSkipCheckInstance
	require.Error(t, err)
	assert.Equal(t, check.ErrSkipCheckInstance, err)
}

// TestBatteryPowerStates tests different power state scenarios
func TestBatteryPowerStates(t *testing.T) {
	tests := []struct {
		name              string
		isCharging        bool
		externalConnected bool
		expectedTags      []string
	}{
		{
			name:              "Discharging on battery",
			isCharging:        false,
			externalConnected: false,
			expectedTags:      []string{"power_state:battery_discharging"},
		},
		{
			name:              "Charging on AC power",
			isCharging:        true,
			externalConnected: true,
			expectedTags:      []string{"power_state:battery_charging", "power_state:battery_power_on_line"},
		},
		{
			name:              "On AC power, not charging (full)",
			isCharging:        false,
			externalConnected: true,
			expectedTags:      []string{"power_state:battery_discharging", "power_state:battery_power_on_line"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFunc := getBatteryInfoFunc
			getBatteryInfoFunc = func() batteryInfo {
				return batteryInfo{
					found:               true,
					cycleCount:          100,
					designCapacity:      8000,
					appleRawMaxCapacity: 7600,
					currentCapacity:     75.0,
					voltage:             12400,
					instantAmperage:     -1800,
					isCharging:          tt.isCharging,
					externalConnected:   tt.externalConnected,
				}
			}
			defer func() {
				getBatteryInfoFunc = originalFunc
			}()

			batteryCheck := &Check{}
			senderManager := mocksender.CreateDefaultDemultiplexer()
			err := batteryCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
			require.NoError(t, err)

			mockSender := mocksender.NewMockSenderWithSenderManager(batteryCheck.ID(), senderManager)
			mockSender.SetupAcceptAll()

			err = batteryCheck.Run()
			require.NoError(t, err)

			mockSender.AssertMetric(t, "Gauge", "system.battery.power_state", 1.0, "", tt.expectedTags)
		})
	}
}

// TestBatteryInfoMethods tests the batteryInfo helper methods
func TestBatteryInfoMethods(t *testing.T) {
	info := batteryInfo{
		found:               true,
		cycleCount:          150,
		designCapacity:      8000,  // mAh
		appleRawMaxCapacity: 7600,  // mAh
		currentCapacity:     84,    // %
		voltage:             12000, // mV
		instantAmperage:     -2000, // mA
	}

	// Test maxCapacityPercent: 7600 / 8000 * 100 = 95%
	assert.Equal(t, 95.0, info.maxCapacityPercent())

	// Test designCapacityMWh: 8000 * 12000 / 1000 = 96000 mWh
	assert.Equal(t, 96000.0, info.designCapacityMWh())

	// Test maxCapacityMWh: 7600 * 12000 / 1000 = 91200 mWh
	assert.Equal(t, 91200.0, info.maxCapacityMWh())

	// Test chargeRateWatts: -2000 * 12000 / 1000 = -24000 mW
	assert.Equal(t, -24000.0, info.chargeRateWatts())

	// Test maxCapacityPercent is capped at 100%
	infoOverCapacity := batteryInfo{
		designCapacity:      8000,
		appleRawMaxCapacity: 9000, // Would be 112.5% without cap
	}
	assert.Equal(t, 100.0, infoOverCapacity.maxCapacityPercent())
}

// TestGetPowerStateTags tests the getPowerStateTags method
func TestGetPowerStateTags(t *testing.T) {
	// Test discharging
	info := batteryInfo{isCharging: false, externalConnected: false}
	assert.Equal(t, []string{"power_state:battery_discharging"}, info.getPowerStateTags())

	// Test charging with external power
	info = batteryInfo{isCharging: true, externalConnected: true}
	assert.Equal(t, []string{"power_state:battery_charging", "power_state:battery_power_on_line"}, info.getPowerStateTags())

	// Test external connected but not charging
	info = batteryInfo{isCharging: false, externalConnected: true}
	assert.Equal(t, []string{"power_state:battery_discharging", "power_state:battery_power_on_line"}, info.getPowerStateTags())
}
