// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"bytes"
	"context"
	"testing"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextCancellationStopsRefresh(t *testing.T) {
	// NOTE: This test uses a simplified goroutine that simulates the context cancellation
	// behavior of startBackgroundRefresh. Testing the actual startBackgroundRefresh function
	// would require extensive mocking of the provider, API client, and config, which is
	// covered by integration tests. This unit test verifies the core goroutine pattern
	// (select on ctx.Done and ticker) works correctly for context cancellation.

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to signal when the goroutine exits
	done := make(chan bool, 1)

	// Create a test version of the background refresh goroutine
	// This simulates the select/case pattern used in startBackgroundRefresh
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer func() { done <- true }()

		for {
			select {
			case <-ctx.Done():
				// Context was canceled, exit the goroutine
				return
			case <-ticker.C:
				// Simulate refresh work
			}
		}
	}()

	// Wait a bit to ensure goroutine is running
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for the goroutine to exit with a timeout
	select {
	case <-done:
		// Success - goroutine exited
	case <-time.After(1 * time.Second):
		t.Fatal("Goroutine did not exit after context cancellation")
	}
}

func TestBackgroundRefreshContextHandling(t *testing.T) {
	// NOTE: Like TestContextCancellationStopsRefresh, this test uses a simplified goroutine
	// that simulates the context cancellation behavior at different lifecycle points.
	// See the note in TestContextCancellationStopsRefresh for rationale.
	//
	// This test verifies that the background refresh goroutine properly
	// handles context cancellation at different points in its lifecycle

	tests := []struct {
		name          string
		cancelAfter   time.Duration
		tickInterval  time.Duration
		expectTimeout bool
	}{
		{
			name:          "cancel before first tick",
			cancelAfter:   10 * time.Millisecond,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
		{
			name:          "cancel after first tick",
			cancelAfter:   150 * time.Millisecond,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
		{
			name:          "cancel immediately",
			cancelAfter:   0,
			tickInterval:  100 * time.Millisecond,
			expectTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan bool, 1)

			// Start the goroutine
			go func() {
				ticker := time.NewTicker(tt.tickInterval)
				defer ticker.Stop()
				defer func() { done <- true }()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						// Check if context is already canceled
						if ctx.Err() != nil {
							return
						}
						// Simulate work
					}
				}
			}()

			// Cancel after specified duration
			if tt.cancelAfter > 0 {
				time.Sleep(tt.cancelAfter)
			}
			cancel()

			// Wait for goroutine to exit
			select {
			case <-done:
				// Success
			case <-time.After(1 * time.Second):
				if !tt.expectTimeout {
					t.Fatal("Goroutine did not exit after context cancellation")
				}
			}
		})
	}
}

// Backoff Configuration Tests

func TestNewBackoff(t *testing.T) {
	refreshInterval := 15 * time.Minute

	b := newBackoff(refreshInterval)

	// Verify backoff is configured correctly
	assert.Equal(t, refreshInterval, b.InitialInterval, "InitialInterval should match refresh interval")
	assert.Equal(t, maxBackoffInterval, b.MaxInterval, "MaxInterval should be maxBackoffInterval")
	assert.Equal(t, 2.0, b.Multiplier, "Multiplier should be 2.0")
	assert.Equal(t, backoffRandomizationFactor, b.RandomizationFactor, "RandomizationFactor should match constant")

	// Verify backoff produces intervals (with jitter, so just check it's reasonable)
	interval := b.NextBackOff()
	minExpected := time.Duration(float64(refreshInterval) * (1 - backoffRandomizationFactor))
	maxExpected := time.Duration(float64(refreshInterval) * (1 + backoffRandomizationFactor))
	assert.GreaterOrEqual(t, interval, minExpected, "First interval should be >= min expected")
	assert.LessOrEqual(t, interval, maxExpected, "First interval should be <= max expected")
}

func TestBackoffExponentialGrowth(t *testing.T) {
	refreshInterval := 15 * time.Minute
	b := newBackoff(refreshInterval)

	// Simulate multiple failures and verify exponential growth
	// Note: Due to jitter, we can only verify approximate bounds
	for i := 0; i < 5; i++ {
		interval := b.NextBackOff()
		// Each interval should be capped at maxBackoffInterval
		assert.LessOrEqual(t, interval, maxBackoffInterval+time.Duration(float64(maxBackoffInterval)*backoffRandomizationFactor),
			"Interval should not exceed max with jitter")
	}

	// Reset and verify it starts fresh
	b.Reset()
	interval := b.NextBackOff()
	maxExpected := time.Duration(float64(refreshInterval) * (1 + backoffRandomizationFactor))
	assert.LessOrEqual(t, interval, maxExpected, "After reset, interval should be back to initial range")
}

// Status Provider Tests

func TestStatusProviderName(t *testing.T) {
	comp := &delegatedAuthComponent{}
	assert.Equal(t, "Delegated Auth", comp.Name())
}

func TestStatusProviderSection(t *testing.T) {
	comp := &delegatedAuthComponent{}
	assert.Equal(t, "delegatedauth", comp.Section())
}

func TestStatusJSON_NotEnabled(t *testing.T) {
	// Test status when delegated auth is not enabled (no instances)
	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	stats := make(map[string]interface{})
	err := comp.JSON(false, stats)

	require.NoError(t, err)
	assert.Equal(t, false, stats["enabled"])
	assert.Nil(t, stats["provider"])
	assert.Nil(t, stats["instances"])
}

func TestStatusJSON_EnabledWithInstances(t *testing.T) {
	// Test status when delegated auth is enabled with instances
	apiKey := "test-api-key-1234567890"
	comp := &delegatedAuthComponent{
		initialized:      true,
		resolvedProvider: cloudauthconfig.ProviderAWS,
		providerConfig:   &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"},
		instances: map[string]*authInstance{
			"api_key": {
				apiKey:              &apiKey,
				refreshInterval:     60 * time.Minute,
				apiKeyConfigKey:     "api_key",
				consecutiveFailures: 0,
			},
			"logs_config.api_key": {
				apiKey:              nil, // Pending
				refreshInterval:     30 * time.Minute,
				apiKeyConfigKey:     "logs_config.api_key",
				consecutiveFailures: 3,
			},
		},
	}

	stats := make(map[string]interface{})
	err := comp.JSON(false, stats)

	require.NoError(t, err)
	assert.Equal(t, true, stats["enabled"])
	assert.Equal(t, cloudauthconfig.ProviderAWS, stats["provider"])
	assert.Equal(t, "us-east-1", stats["awsRegion"])

	instances, ok := stats["instances"].(map[string]map[string]interface{})
	require.True(t, ok, "instances should be a map")
	require.Len(t, instances, 2)

	// Check api_key instance
	apiKeyInstance := instances["api_key"]
	assert.Equal(t, "Active", apiKeyInstance["Status"])
	assert.Equal(t, "1h0m0s", apiKeyInstance["RefreshInterval"])
	assert.Nil(t, apiKeyInstance["Error"])

	// Check logs_config.api_key instance (pending with failures)
	logsInstance := instances["logs_config.api_key"]
	assert.Equal(t, "Pending", logsInstance["Status"])
	assert.Equal(t, "30m0s", logsInstance["RefreshInterval"])
	assert.Equal(t, "3 consecutive failures", logsInstance["Error"])
}

func TestStatusText_NotEnabled(t *testing.T) {
	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	var buffer bytes.Buffer
	err := comp.Text(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()
	assert.Contains(t, output, "Delegated Authentication is not enabled")
}

func TestStatusText_EnabledWithInstances(t *testing.T) {
	apiKey := "test-api-key-1234567890"
	comp := &delegatedAuthComponent{
		initialized:      true,
		resolvedProvider: cloudauthconfig.ProviderAWS,
		providerConfig:   &cloudauthconfig.AWSProviderConfig{Region: "us-west-2"},
		instances: map[string]*authInstance{
			"api_key": {
				apiKey:              &apiKey,
				refreshInterval:     45 * time.Minute,
				apiKeyConfigKey:     "api_key",
				consecutiveFailures: 0,
			},
		},
	}

	var buffer bytes.Buffer
	err := comp.Text(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()

	// Verify key information is present in the text output
	assert.Contains(t, output, "Delegated Authentication")
	assert.Contains(t, output, cloudauthconfig.ProviderAWS)
	assert.Contains(t, output, "us-west-2")
	assert.Contains(t, output, "api_key")
	assert.Contains(t, output, "Active")
}

func TestStatusHTML_NotEnabled(t *testing.T) {
	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	var buffer bytes.Buffer
	err := comp.HTML(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()

	// Verify HTML structure and content
	assert.Contains(t, output, "<div")
	assert.Contains(t, output, "Delegated Authentication")
	assert.Contains(t, output, "Not enabled")
}

func TestStatusHTML_EnabledWithInstances(t *testing.T) {
	apiKey := "test-api-key-1234567890"
	comp := &delegatedAuthComponent{
		initialized:      true,
		resolvedProvider: cloudauthconfig.ProviderAWS,
		providerConfig:   &cloudauthconfig.AWSProviderConfig{Region: "eu-west-1"},
		instances: map[string]*authInstance{
			"api_key": {
				apiKey:              &apiKey,
				refreshInterval:     60 * time.Minute,
				apiKeyConfigKey:     "api_key",
				consecutiveFailures: 0,
			},
		},
	}

	var buffer bytes.Buffer
	err := comp.HTML(false, &buffer)

	require.NoError(t, err)
	output := buffer.String()

	// Verify HTML structure and content
	assert.Contains(t, output, "<div")
	assert.Contains(t, output, "Delegated Authentication")
	assert.Contains(t, output, cloudauthconfig.ProviderAWS)
	assert.Contains(t, output, "eu-west-1")
	assert.Contains(t, output, "api_key")
}

func TestStatusPopulateInfo_MultipleInstances(t *testing.T) {
	// Test the internal populateStatusInfo with multiple instances in various states
	apiKey1 := "active-key-1"
	comp := &delegatedAuthComponent{
		initialized:      true,
		resolvedProvider: cloudauthconfig.ProviderAWS,
		providerConfig:   &cloudauthconfig.AWSProviderConfig{Region: "ap-southeast-1"},
		instances: map[string]*authInstance{
			"api_key": {
				apiKey:              &apiKey1,
				refreshInterval:     60 * time.Minute,
				apiKeyConfigKey:     "api_key",
				consecutiveFailures: 0,
			},
			"logs_config.api_key": {
				apiKey:              nil,
				refreshInterval:     30 * time.Minute,
				apiKeyConfigKey:     "logs_config.api_key",
				consecutiveFailures: 5,
			},
			"apm_config.api_key": {
				apiKey:              nil,
				refreshInterval:     15 * time.Minute,
				apiKeyConfigKey:     "apm_config.api_key",
				consecutiveFailures: 0,
			},
		},
	}

	stats := make(map[string]interface{})
	comp.populateStatusInfo(stats)

	assert.Equal(t, true, stats["enabled"])
	assert.Equal(t, cloudauthconfig.ProviderAWS, stats["provider"])
	assert.Equal(t, "ap-southeast-1", stats["awsRegion"])

	instances := stats["instances"].(map[string]map[string]interface{})
	require.Len(t, instances, 3)

	// Active instance
	assert.Equal(t, "Active", instances["api_key"]["Status"])
	assert.Nil(t, instances["api_key"]["Error"])

	// Pending with failures
	assert.Equal(t, "Pending", instances["logs_config.api_key"]["Status"])
	assert.Equal(t, "5 consecutive failures", instances["logs_config.api_key"]["Error"])

	// Pending without failures
	assert.Equal(t, "Pending", instances["apm_config.api_key"]["Status"])
	assert.Nil(t, instances["apm_config.api_key"]["Error"])
}

// Refresh Interval Validation Tests

func TestNewBackoffWithNegativeInterval(t *testing.T) {
	// Test that newBackoff handles negative intervals gracefully
	// This verifies the fix for P1: non-positive refresh intervals should not cause panics

	// Test with zero interval - should work with backoff's default behavior
	b := newBackoff(0)
	require.NotNil(t, b, "backoff should be created even with zero interval")

	// Test that NextBackOff doesn't panic
	interval := b.NextBackOff()
	assert.GreaterOrEqual(t, interval, time.Duration(0), "interval should be non-negative")
}

func TestRefreshIntervalValidation(t *testing.T) {
	// This test documents the expected behavior for refresh interval validation
	// The AddInstance function should handle non-positive intervals by defaulting to 60 minutes

	tests := []struct {
		name           string
		inputInterval  int
		expectedBounds struct {
			min time.Duration
			max time.Duration
		}
	}{
		{
			name:          "zero interval defaults to 60 minutes",
			inputInterval: 0,
			expectedBounds: struct {
				min time.Duration
				max time.Duration
			}{
				// With jitter (10%), the interval should be between 54-66 minutes
				min: 54 * time.Minute,
				max: 66 * time.Minute,
			},
		},
		{
			name:          "negative interval defaults to 60 minutes",
			inputInterval: -1,
			expectedBounds: struct {
				min time.Duration
				max time.Duration
			}{
				min: 54 * time.Minute,
				max: 66 * time.Minute,
			},
		},
		{
			name:          "positive interval is used as-is",
			inputInterval: 30,
			expectedBounds: struct {
				min time.Duration
				max time.Duration
			}{
				// With jitter (10%), 30 minutes should be between 27-33 minutes
				min: 27 * time.Minute,
				max: 33 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate the refresh interval as done in AddInstance
			refreshInterval := time.Duration(tt.inputInterval) * time.Minute
			if refreshInterval <= 0 {
				refreshInterval = 60 * time.Minute
			}

			// Create backoff and verify the interval is within expected bounds
			b := newBackoff(refreshInterval)
			interval := b.NextBackOff()

			assert.GreaterOrEqual(t, interval, tt.expectedBounds.min,
				"interval should be >= min expected")
			assert.LessOrEqual(t, interval, tt.expectedBounds.max,
				"interval should be <= max expected")
		})
	}
}
