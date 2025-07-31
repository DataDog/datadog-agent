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
	originalInitialDelay := DefaultInitialDelay
	originalRecurringInterval := DefaultRecurringInterval
	defer func() {
		DefaultInitialDelay = originalInitialDelay
		DefaultRecurringInterval = originalRecurringInterval
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

	// Cancel the context to stop the goroutine
	cancel()

	// Give some time for the goroutine to clean up
	time.Sleep(10 * time.Millisecond)
}

func TestSetDefaultInitialDelay(t *testing.T) {
	// Save original value
	originalDelay := DefaultInitialDelay
	defer func() {
		DefaultInitialDelay = originalDelay
	}()

	// Test setting a new delay
	newDelay := 5 * time.Minute
	setDefaultInitialDelay(newDelay)
	assert.Equal(t, newDelay, DefaultInitialDelay)

	// Test setting zero delay
	setDefaultInitialDelay(0)
	assert.Equal(t, time.Duration(0), DefaultInitialDelay)

	// Test setting negative delay (should work as it's just a time.Duration)
	setDefaultInitialDelay(-1 * time.Second)
	assert.Equal(t, -1*time.Second, DefaultInitialDelay)
}

func TestSetDefaultRecurringInterval(t *testing.T) {
	// Save original value
	originalInterval := DefaultRecurringInterval
	defer func() {
		DefaultRecurringInterval = originalInterval
	}()

	// Test setting a new interval
	newInterval := 2 * time.Hour
	setDefaultRecurringInterval(newInterval)
	assert.Equal(t, newInterval, DefaultRecurringInterval)

	// Test setting zero interval
	setDefaultRecurringInterval(0)
	assert.Equal(t, time.Duration(0), DefaultRecurringInterval)

	// Test setting negative interval (should work as it's just a time.Duration)
	setDefaultRecurringInterval(-1 * time.Minute)
	assert.Equal(t, -1*time.Minute, DefaultRecurringInterval)
}

func TestScheduleHostnameDriftChecksWithCustomTiming(t *testing.T) {
	// Clear cache before test
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Delete(cacheHostnameKey)

	// Create test data
	hostnameData := Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}

	// Save original values
	originalInitialDelay := DefaultInitialDelay
	originalRecurringInterval := DefaultRecurringInterval
	defer func() {
		DefaultInitialDelay = originalInitialDelay
		DefaultRecurringInterval = originalRecurringInterval
	}()

	// Set custom timing for testing
	setDefaultInitialDelay(5 * time.Millisecond)
	setDefaultRecurringInterval(10 * time.Millisecond)

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

	// Cancel the context to stop the goroutine
	cancel()

	// Give some time for the goroutine to clean up
	time.Sleep(10 * time.Millisecond)
}

func TestDriftConstants(t *testing.T) {
	// Test that the drift state constants are properly defined
	assert.NotEmpty(t, hostnameChanged)
	assert.NotEmpty(t, providerChanged)
	assert.NotEmpty(t, hostnameProviderChanged)
	assert.NotEmpty(t, noDrift)
}

func TestDefaultTimingConstants(t *testing.T) {
	// Test that the default timing constants are properly defined
	assert.Equal(t, 20*time.Minute, DefaultInitialDelay)
	assert.Equal(t, 6*time.Hour, DefaultRecurringInterval)
}

// Benchmark tests for performance
func BenchmarkDetermineDriftState(b *testing.B) {
	oldData := Data{Hostname: "old-host", Provider: "old-provider"}
	newData := Data{Hostname: "new-host", Provider: "new-provider"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		determineDriftState(oldData, newData)
	}
}
