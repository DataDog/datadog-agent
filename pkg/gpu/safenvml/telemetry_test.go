// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// mockFailingNvmlNew returns a mock that always fails initialization
func mockFailingNvmlNew(_ ...nvml.LibraryOption) nvml.Interface {
	return &nvmlmock.Interface{
		InitFunc: func() nvml.Return {
			return nvml.ERROR_UNKNOWN
		},
	}
}

// mockSuccessfulNvmlNew returns a mock that successfully initializes
func mockSuccessfulNvmlNew(_ ...nvml.LibraryOption) nvml.Interface {
	return &nvmlmock.Interface{
		InitFunc: func() nvml.Return {
			return nvml.SUCCESS
		},
		ExtensionsFunc: func() nvml.ExtendedInterface {
			return &nvmlmock.ExtendedInterface{
				LookupSymbolFunc: func(_ string) error {
					return nil
				},
			}
		},
	}
}

func TestNvmlStateTelemetry_CheckUnavailable(t *testing.T) {
	WithMockNvmlNewFunc(t, mockFailingNvmlNew)
	telemetryMock := fxutil.Test[telemetryimpl.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTelemetry(telemetryMock)

	// First check - should increment error counter but not set unavailable gauge
	tracker.Check()

	// Verify error counter increased
	errorMetrics, err := telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(1), errorMetrics[0].Value())

	// Library should not still be marked as unavailable
	gaugeMetrics, err := telemetryMock.GetGaugeMetric("gpu__nvml", "library_unavailable")
	require.NoError(t, err)
	require.Len(t, gaugeMetrics, 1)
	assert.Equal(t, float64(0), gaugeMetrics[0].Value())

	// Simulate time passing beyond threshold (threshold is 5 minutes)
	tracker.firstCheckTime = time.Now().Add(-2 * nvmlUnavailableThreshold)

	// Second check - should now mark as unavailable
	tracker.Check()

	// Verify error counter increased again
	errorMetrics, err = telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(2), errorMetrics[0].Value())

	// Verify gauge is now set to 1
	gaugeMetrics, err = telemetryMock.GetGaugeMetric("gpu__nvml", "library_unavailable")
	require.NoError(t, err)
	require.Len(t, gaugeMetrics, 1)
	assert.Equal(t, float64(1), gaugeMetrics[0].Value())
}

func TestNvmlStateTelemetry_CheckMultipleErrors(t *testing.T) {
	WithMockNvmlNewFunc(t, mockFailingNvmlNew)
	telemetryMock := fxutil.Test[telemetryimpl.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTelemetry(telemetryMock)

	// Multiple checks before threshold
	for i := 0; i < 5; i++ {
		tracker.Check()
	}

	// Verify error counter increased
	errorMetrics, err := telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(5), errorMetrics[0].Value())

	// Library should not still be marked as unavailable
	gaugeMetrics, err := telemetryMock.GetGaugeMetric("gpu__nvml", "library_unavailable")
	require.NoError(t, err)
	require.Len(t, gaugeMetrics, 1)
	assert.Equal(t, float64(0), gaugeMetrics[0].Value())
}

func TestNvmlStateTelemetry_CheckRecovery(t *testing.T) {
	telemetryMock := fxutil.Test[telemetryimpl.Mock](t, telemetryimpl.MockModule())
	tracker := NewNvmlStateTelemetry(telemetryMock)

	// Start with failing initialization
	WithMockNvmlNewFunc(t, mockFailingNvmlNew)

	// First check - should fail and increment error counter
	tracker.Check()

	// Verify error counter increased
	errorMetrics, err := telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(1), errorMetrics[0].Value())

	// Simulate time passing beyond threshold
	tracker.firstCheckTime = time.Now().Add(-2 * nvmlUnavailableThreshold)

	// Second check - should mark as unavailable
	tracker.Check()

	// Verify gauge is set to 1
	gaugeMetrics, err := telemetryMock.GetGaugeMetric("gpu__nvml", "library_unavailable")
	require.NoError(t, err)
	require.Len(t, gaugeMetrics, 1)
	assert.Equal(t, float64(1), gaugeMetrics[0].Value())

	// Now simulate recovery by switching to successful initialization
	resetSingleton()
	nvmlNewFunc = mockSuccessfulNvmlNew

	// Check again - should now succeed
	tracker.Check()

	// Verify gauge is reset to 0
	gaugeMetrics, err = telemetryMock.GetGaugeMetric("gpu__nvml", "library_unavailable")
	require.NoError(t, err)
	require.Len(t, gaugeMetrics, 1)
	assert.Equal(t, float64(0), gaugeMetrics[0].Value(), "Gauge should be reset to 0 after recovery")

	// Verify error counter did not increase (recovery doesn't count as error)
	errorMetrics, err = telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(2), errorMetrics[0].Value(), "Error counter should not increase after successful recovery")
}

func TestNvmlStateTelemetry_StartStop(t *testing.T) {
	WithMockNvmlNewFunc(t, mockFailingNvmlNew)
	telemetryMock := fxutil.Test[telemetryimpl.Mock](t, telemetryimpl.MockModule())
	// Use a short interval for testing
	tracker := NewNvmlStateTelemetry(telemetryMock)
	tracker.checkInterval = 100 * time.Millisecond

	// Start the tracker
	tracker.Start()

	// Wait for a few checks to occur
	time.Sleep(350 * time.Millisecond)

	// Verify some checks occurred (error count should be > 0 since NVML is not available in test)
	errorMetrics, err := telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	errorCount := errorMetrics[0].Value()

	assert.Greater(t, errorCount, float64(0), "Expected at least one check to have occurred")
	assert.LessOrEqual(t, errorCount, float64(5), "Expected no more than 5 checks in 350ms with 100ms interval")

	// Stop should cleanly shut down
	tracker.Stop()

	// After stop, no more checks should occur
	errorMetrics, err = telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	errorCountAfterStop := errorMetrics[0].Value()

	time.Sleep(200 * time.Millisecond)

	errorMetrics, err = telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	errorCountAfterWait := errorMetrics[0].Value()

	assert.Equal(t, errorCountAfterStop, errorCountAfterWait, "No checks should occur after Stop()")
}

func TestNvmlStateTelemetry_StartPerformsImmediateCheck(t *testing.T) {
	WithMockNvmlNewFunc(t, mockFailingNvmlNew)
	telemetryMock := fxutil.Test[telemetryimpl.Mock](t, telemetryimpl.MockModule())
	// Use a long interval so we can verify the immediate check
	tracker := NewNvmlStateTelemetry(telemetryMock)
	tracker.checkInterval = 10 * time.Second

	// Start the tracker
	tracker.Start()
	t.Cleanup(tracker.Stop)

	// Give it a moment to perform the initial check
	time.Sleep(50 * time.Millisecond)

	// Verify the immediate check occurred
	errorMetrics, err := telemetryMock.GetCountMetric("gpu__nvml", "init_errors")
	require.NoError(t, err)
	require.Len(t, errorMetrics, 1)
	assert.Equal(t, float64(1), errorMetrics[0].Value(), "Expected exactly one check immediately after Start()")
}
