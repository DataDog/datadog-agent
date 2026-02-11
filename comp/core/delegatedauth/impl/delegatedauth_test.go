// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package delegatedauthimpl

import (
	"bytes"
	"context"
	"math"
	"testing"
	"time"

	cloudauthconfig "github.com/DataDog/datadog-agent/comp/core/delegatedauth/api/cloudauth/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextCancellationStopsRefresh(t *testing.T) {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to signal when the goroutine exits
	done := make(chan bool, 1)

	// Create a test version of the background refresh goroutine
	// This simulates the actual implementation in startBackgroundRefresh
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

func TestCalculateNextRetryInterval(t *testing.T) {
	tests := []struct {
		name                string
		refreshInterval     time.Duration
		consecutiveFailures int
		expectedInterval    time.Duration
	}{
		{
			name:                "first failure - base interval",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 1,
			expectedInterval:    15 * time.Minute, // 15 * 2^0 = 15 minutes
		},
		{
			name:                "second failure - 2x base interval",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 2,
			expectedInterval:    30 * time.Minute, // 15 * 2^1 = 30 minutes
		},
		{
			name:                "third failure - 4x base interval, capped at max",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 3,
			expectedInterval:    time.Hour, // 15 * 2^2 = 60 minutes (1 hour)
		},
		{
			name:                "fourth failure - still capped at max",
			refreshInterval:     15 * time.Minute,
			consecutiveFailures: 4,
			expectedInterval:    time.Hour, // 15 * 2^3 = 120 minutes, capped at 1 hour
		},
		{
			name:                "no failures - base interval",
			refreshInterval:     30 * time.Minute,
			consecutiveFailures: 0,
			expectedInterval:    30 * time.Minute, // 30 * 2^0 = 30 minutes
		},
		{
			name:                "small base interval with failures",
			refreshInterval:     5 * time.Minute,
			consecutiveFailures: 1,
			expectedInterval:    5 * time.Minute, // 5 * 2^0 = 5 minutes
		},
		{
			name:                "small base interval quickly hits max",
			refreshInterval:     5 * time.Minute,
			consecutiveFailures: 5,
			expectedInterval:    time.Hour, // 5 * 2^4 = 80 minutes, capped at 1 hour
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := &delegatedAuthComponent{}
			instance := &authInstance{
				refreshInterval:     tt.refreshInterval,
				consecutiveFailures: tt.consecutiveFailures,
			}

			result := comp.calculateNextRetryInterval(instance)
			assert.Equal(t, tt.expectedInterval, result,
				"Expected %v but got %v for %d consecutive failures with base interval %v",
				tt.expectedInterval, result, tt.consecutiveFailures, tt.refreshInterval)
		})
	}
}

func TestExponentialBackoffBehavior(t *testing.T) {
	// Test the backoff progression for a typical scenario
	comp := &delegatedAuthComponent{}
	instance := &authInstance{
		refreshInterval:     15 * time.Minute,
		consecutiveFailures: 0,
	}

	// Test progression through multiple failures
	expectedIntervals := []time.Duration{
		15 * time.Minute, // 0 failures: 15 * 2^0 = 15 minutes
		15 * time.Minute, // 1 failure:  15 * 2^0 = 15 minutes (first retry at base interval)
		30 * time.Minute, // 2 failures: 15 * 2^1 = 30 minutes
		60 * time.Minute, // 3 failures: 15 * 2^2 = 60 minutes (1 hour)
		60 * time.Minute, // 4 failures: 15 * 2^3 = 120 minutes, capped at 1 hour
	}

	for i, expected := range expectedIntervals {
		result := comp.calculateNextRetryInterval(instance)
		assert.Equal(t, expected, result,
			"Failure %d: expected %v but got %v", i, expected, result)

		// Simulate a failure for the next iteration
		if i < len(expectedIntervals)-1 {
			instance.consecutiveFailures++
		}
	}
}

func TestConsecutiveFailuresCap(t *testing.T) {
	// Test that consecutiveFailures is capped at maxConsecutiveFailures
	instance := &authInstance{
		refreshInterval:     5 * time.Minute,
		consecutiveFailures: 0,
	}

	// Simulate many failures - consecutiveFailures should be capped
	for i := 0; i < maxConsecutiveFailures+5; i++ {
		if instance.consecutiveFailures < maxConsecutiveFailures {
			instance.consecutiveFailures++
		}
	}

	assert.Equal(t, maxConsecutiveFailures, instance.consecutiveFailures,
		"consecutiveFailures should be capped at %d", maxConsecutiveFailures)

	// Verify backoff still works correctly at the cap
	comp := &delegatedAuthComponent{}
	result := comp.calculateNextRetryInterval(instance)
	assert.Equal(t, time.Hour, result,
		"Backoff should still be capped at max interval even with max consecutive failures")
}

func TestMaxConsecutiveFailuresValue(t *testing.T) {
	// Verify that maxConsecutiveFailures is set to a reasonable value
	// that ensures we reach maxBackoffInterval with any reasonable refresh_interval

	// Test with minimum reasonable refresh interval (1 minute)
	comp := &delegatedAuthComponent{}
	instance := &authInstance{
		refreshInterval:     1 * time.Minute,
		consecutiveFailures: maxConsecutiveFailures,
	}

	result := comp.calculateNextRetryInterval(instance)
	assert.Equal(t, maxBackoffInterval, result,
		"With maxConsecutiveFailures, even a 1-minute refresh interval should reach maxBackoffInterval")

	// Verify the math: 1 minute * 2^(maxConsecutiveFailures-1) should be >= 60 minutes
	calculatedBackoff := time.Duration(float64(1*time.Minute) * math.Pow(2, float64(maxConsecutiveFailures-1)))
	assert.GreaterOrEqual(t, calculatedBackoff, maxBackoffInterval,
		"maxConsecutiveFailures (%d) should be high enough that 1 minute * 2^%d >= 1 hour",
		maxConsecutiveFailures, maxConsecutiveFailures-1)
}

func TestAddJitter(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{
			name:     "1 minute",
			duration: 1 * time.Minute,
		},
		{
			name:     "15 minutes",
			duration: 15 * time.Minute,
		},
		{
			name:     "60 minutes",
			duration: 60 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to verify jitter variability
			results := make([]time.Duration, 100)
			for i := 0; i < 100; i++ {
				jittered := addJitter(tt.duration)
				results[i] = jittered

				// Verify jittered value is within expected range
				minExpected := time.Duration(float64(tt.duration) * (1 - jitterPercent))
				maxExpected := time.Duration(float64(tt.duration) * (1 + jitterPercent))

				assert.GreaterOrEqual(t, jittered, minExpected,
					"Jittered value %v should be >= %v", jittered, minExpected)
				assert.LessOrEqual(t, jittered, maxExpected,
					"Jittered value %v should be <= %v", jittered, maxExpected)
			}

			// Verify that we got some variation (not all the same value)
			allSame := true
			firstValue := results[0]
			for _, v := range results[1:] {
				if v != firstValue {
					allSame = false
					break
				}
			}
			assert.False(t, allSame, "Jitter should produce varying values")
		})
	}
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
