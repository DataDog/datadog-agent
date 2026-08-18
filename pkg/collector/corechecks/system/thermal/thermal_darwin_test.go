// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package thermal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

// setupMocks makes getThermalReadingFunc return the supplied reading.
// Returns a cleanup function that should be deferred.
func setupMocks(reading thermalReading) func() {
	original := getThermalReadingFunc
	getThermalReadingFunc = func() thermalReading {
		return reading
	}
	return func() {
		getThermalReadingFunc = original
	}
}

// newTestCheck builds a configured check wired to a mock sender.
func newTestCheck(t *testing.T) (*thermalCheck, *mocksender.MockSender) {
	t.Helper()

	check := newCheck().(*thermalCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider"))

	mockSender := mocksender.NewMockSenderWithSenderManager(check.ID(), senderManager)
	mockSender.SetupAcceptAll()

	return check, mockSender
}

func float64Ptr(v float64) *float64 { return &v }
func intPtr(v int) *int             { return &v }

// TestRunSubmitsAllMetricsWhenEverySensorAvailable is the positive path: every
// sensor reads, so all five gauges are submitted with the expected tags.
func TestRunSubmitsAllMetricsWhenEverySensorAvailable(t *testing.T) {
	defer setupMocks(convertCThermalInfo(testCThermalInfoFull))()

	check, mockSender := newTestCheck(t)
	require.NoError(t, check.Run())

	mockSender.AssertMetric(t, "Gauge", "system.thermal.temperature.cpu", 61.5, "", []string{"macos", "smc", "cpu"})
	mockSender.AssertMetric(t, "Gauge", "system.thermal.temperature.gpu", 48.25, "", []string{"macos", "smc", "gpu"})
	mockSender.AssertMetric(t, "Gauge", "system.thermal.temperature.ssd", 39.0, "", []string{"macos", "smc", "ssd"})
	mockSender.AssertMetric(t, "Gauge", "system.thermal.temperature.battery", 31.75, "", []string{"macos", "smc", "battery"})
	mockSender.AssertMetric(t, "Gauge", "system.thermal.pressure_level", 2.0, "", []string{"macos", "pressure_level:heavy"})

	mockSender.AssertNumberOfCalls(t, "Gauge", 5)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestRunSubmitsNoMetricsWhenNoSensorAvailable is the negative path: nothing
// reads, so no gauge is submitted and the check still succeeds.
func TestRunSubmitsNoMetricsWhenNoSensorAvailable(t *testing.T) {
	defer setupMocks(convertCThermalInfo(testCThermalInfoEmpty))()

	check, mockSender := newTestCheck(t)
	require.NoError(t, check.Run())

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestRunSkipsOnlyUnavailableSensors covers partial availability: a sensor that
// did not read is skipped without suppressing the ones that did.
func TestRunSkipsOnlyUnavailableSensors(t *testing.T) {
	defer setupMocks(thermalReading{
		smc: smcReading{cpu: float64Ptr(55.0)},
	})()

	check, mockSender := newTestCheck(t)
	require.NoError(t, check.Run())

	mockSender.AssertMetric(t, "Gauge", "system.thermal.temperature.cpu", 55.0, "", []string{"macos", "smc", "cpu"})
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}

// TestRunSubmitsPressureLevelWithoutTemperatures covers the inverse partial
// case: the notification resolved but no SMC key did.
func TestRunSubmitsPressureLevelWithoutTemperatures(t *testing.T) {
	defer setupMocks(thermalReading{thermalLevel: intPtr(0)})()

	check, mockSender := newTestCheck(t)
	require.NoError(t, check.Run())

	mockSender.AssertMetric(t, "Gauge", "system.thermal.pressure_level", 0.0, "", []string{"macos", "pressure_level:nominal"})
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
}

func TestOptionalFloat(t *testing.T) {
	value := optionalFloat(testCOptionalFloatSet)
	require.NotNil(t, value)
	assert.Equal(t, 42.5, *value)

	assert.Nil(t, optionalFloat(testCOptionalFloatUnset))
}

func TestOptionalInt(t *testing.T) {
	value := optionalInt(testCOptionalIntSet)
	require.NotNil(t, value)
	assert.Equal(t, 3, *value)

	assert.Nil(t, optionalInt(testCOptionalIntUnset))
}

func TestConvertCThermalInfo(t *testing.T) {
	full := convertCThermalInfo(testCThermalInfoFull)
	require.NotNil(t, full.smc.cpu)
	require.NotNil(t, full.smc.gpu)
	require.NotNil(t, full.smc.ssd)
	require.NotNil(t, full.smc.battery)
	require.NotNil(t, full.thermalLevel)
	assert.Equal(t, 61.5, *full.smc.cpu)
	assert.Equal(t, 48.25, *full.smc.gpu)
	assert.Equal(t, 39.0, *full.smc.ssd)
	assert.Equal(t, 31.75, *full.smc.battery)
	assert.Equal(t, 2, *full.thermalLevel)

	empty := convertCThermalInfo(testCThermalInfoEmpty)
	assert.Nil(t, empty.smc.cpu)
	assert.Nil(t, empty.smc.gpu)
	assert.Nil(t, empty.smc.ssd)
	assert.Nil(t, empty.smc.battery)
	assert.Nil(t, empty.thermalLevel)
}

func TestThermalPressureLevelName(t *testing.T) {
	tests := []struct {
		name     string
		level    int
		expected string
	}{
		{name: "nominal", level: 0, expected: "nominal"},
		{name: "moderate", level: 1, expected: "moderate"},
		{name: "heavy", level: 2, expected: "heavy"},
		{name: "trapping", level: 3, expected: "trapping"},
		{name: "sleeping", level: 4, expected: "sleeping"},
		{name: "negative is unknown", level: -1, expected: "unknown"},
		{name: "just above range is unknown", level: 5, expected: "unknown"},
		{name: "large value is unknown", level: 9999, expected: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, thermalPressureLevelName(test.level))
		})
	}
}
