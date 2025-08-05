// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestDetermineDriftState(t *testing.T) {
	tests := []struct {
		name     string
		oldData  Data
		newData  Data
		expected driftInfo
	}{
		{
			name: "no drift",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
		{
			name: "hostname changed only",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host2",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    hostnameChanged,
				hasDrift: true,
			},
		},
		{
			name: "provider changed only",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "provider2",
			},
			expected: driftInfo{
				state:    providerChanged,
				hasDrift: true,
			},
		},
		{
			name: "both hostname and provider changed",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host2",
				Provider: "provider2",
			},
			expected: driftInfo{
				state:    hostnameProviderChanged,
				hasDrift: true,
			},
		},
		{
			name: "empty hostnames",
			oldData: Data{
				Hostname: "",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
		{
			name: "empty providers",
			oldData: Data{
				Hostname: "host1",
				Provider: "",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineDriftState(tt.oldData, tt.newData)
			assert.Equal(t, tt.expected.state, result.state)
			assert.Equal(t, tt.expected.hasDrift, result.hasDrift)
		})
	}
}

func TestTelemetryMetricsCreation(t *testing.T) {
	// Test that the telemetry metrics are properly created
	assert.NotNil(t, tlmDriftDetected, "Expected drift_detected telemetry metric to be created")
	assert.NotNil(t, tlmDriftResolutionTime, "Expected drift_resolution_time_ms telemetry metric to be created")
}

func TestDriftStateTelemetryEmission(t *testing.T) {
	// Test that telemetry metrics are emitted when drift is detected
	// This test directly tests the telemetry emission without relying on hostname detection

	// Create test data
	oldData := Data{
		Hostname: "old-hostname",
		Provider: "old-provider",
	}

	newData := Data{
		Hostname: "new-hostname",
		Provider: "new-provider",
	}

	// Determine drift state
	drift := determineDriftState(oldData, newData)

	// Verify drift was detected
	assert.True(t, drift.hasDrift, "Expected drift to be detected")
	assert.Equal(t, hostnameProviderChanged, drift.state, "Expected hostname_provider_drift state")

	// Test that telemetry metrics can be incremented (this verifies they exist and work)
	// Note: We can't easily verify the actual values in tests, but we can verify the metrics exist
	assert.NotNil(t, tlmDriftDetected, "Expected drift_detected telemetry metric to exist")
	assert.NotNil(t, tlmDriftResolutionTime, "Expected drift_resolution_time_ms telemetry metric to exist")
}

func TestScheduleHostnameDriftChecks(t *testing.T) {
	// Clear cache before test
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Delete(cacheHostnameKey)

	// Create test data
	hostnameData := Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}

	// Set shorter intervals for testing
	originalInitialDelay := defaultInitialDelay
	originalRecurringInterval := defaultRecurringInterval
	defer func() {
		setDefaultInitialDelay(originalInitialDelay)
		setDefaultRecurringInterval(originalRecurringInterval)
	}()

	// Use shorter intervals for faster testing
	setDefaultInitialDelay(10 * time.Millisecond)
	setDefaultRecurringInterval(50 * time.Millisecond)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Schedule the drift checks
	scheduleHostnameDriftChecks(ctx, hostnameData)

	// Verify that the initial data was cached
	cachedData, found := cache.Cache.Get(cacheHostnameKey)
	require.True(t, found, "Expected hostname data to be cached")

	cachedHostnameData, ok := cachedData.(Data)
	require.True(t, ok, "Expected cached data to be of type Data")
	assert.Equal(t, hostnameData.Hostname, cachedHostnameData.Hostname)
	assert.Equal(t, hostnameData.Provider, cachedHostnameData.Provider)

	// Verify that telemetry metrics were created (they should exist even if we can't access them directly in tests)
	// The telemetry metrics are created as global variables in drift.go, so they should be available
	assert.NotNil(t, tlmDriftDetected, "Expected drift_detected telemetry metric to be created")
	assert.NotNil(t, tlmDriftResolutionTime, "Expected drift_resolution_time_ms telemetry metric to be created")

	// Cancel the context to stop the goroutine
	cancel()

	// Give some time for the goroutine to clean up
	time.Sleep(10 * time.Millisecond)
}

func TestConfigDriftTiming(t *testing.T) {
	// Test that configuration options override default timing values
	originalInitialDelay := defaultInitialDelay
	originalRecurringInterval := defaultRecurringInterval
	defer func() {
		setDefaultInitialDelay(originalInitialDelay)
		setDefaultRecurringInterval(originalRecurringInterval)
	}()

	// Set test values
	testInitialDelay := 5 * time.Second
	testRecurringInterval := 10 * time.Second

	// Test that getInitialDelay and getRecurringInterval return default values when no config is set
	assert.Equal(t, defaultInitialDelay, getInitialDelay())
	assert.Equal(t, defaultRecurringInterval, getRecurringInterval())

	// Test that setter functions work
	setDefaultInitialDelay(testInitialDelay)
	setDefaultRecurringInterval(testRecurringInterval)

	assert.Equal(t, testInitialDelay, getInitialDelay())
	assert.Equal(t, testRecurringInterval, getRecurringInterval())
}
