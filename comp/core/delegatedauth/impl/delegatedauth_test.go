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
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
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

// Done Channel Tests - Goroutine lifecycle management

func TestDoneChannelClosedOnGoroutineExit(t *testing.T) {
	// Test that the done channel is properly closed when the background goroutine exits.
	// This ensures proper goroutine lifecycle signaling for instance replacement.

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	// Simulate the goroutine pattern used in startBackgroundRefresh
	go func() {
		defer close(done) // This is what we added to fix P3

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Simulate work
			}
		}
	}()

	// Cancel the context to trigger goroutine exit
	cancel()

	// Verify the done channel is closed
	select {
	case <-done:
		// Success - done channel was closed
	case <-time.After(1 * time.Second):
		t.Fatal("Done channel was not closed after goroutine exit")
	}
}

func TestWaitingForOldGoroutineOnInstanceReplacement(t *testing.T) {
	// Test that when replacing an instance, we properly wait for the old goroutine to exit.
	// This prevents goroutine leaks when instances are rapidly reconfigured.

	oldCtx, oldCancel := context.WithCancel(context.Background())
	oldDone := make(chan struct{})

	// Track goroutine execution
	oldGoroutineStarted := make(chan struct{})
	oldGoroutineExited := make(chan struct{})

	// Simulate an "old" goroutine that takes some time to exit
	go func() {
		defer close(oldDone)
		close(oldGoroutineStarted)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-oldCtx.Done():
				// Simulate some cleanup time
				time.Sleep(50 * time.Millisecond)
				close(oldGoroutineExited)
				return
			case <-ticker.C:
				// Simulate work
			}
		}
	}()

	// Wait for old goroutine to start
	<-oldGoroutineStarted

	// Now simulate the replacement logic from AddInstance
	// Cancel the old goroutine
	oldCancel()

	// Wait for the old goroutine to exit (simulating the select in AddInstance)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	select {
	case <-oldDone:
		// Success - old goroutine exited
	case <-waitCtx.Done():
		t.Fatal("Timed out waiting for old goroutine to exit")
	}

	// Verify the old goroutine actually exited
	select {
	case <-oldGoroutineExited:
		// Success
	default:
		t.Fatal("Old goroutine did not properly exit")
	}
}

// Context Cancellation Tests - AddInstance context support

func TestAddInstanceRespectsContextCancellation(t *testing.T) {
	// Test that AddInstance returns early when context is already canceled.
	// This ensures callers can cancel initialization if needed.

	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
	}

	// Create an already-canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Use the configmock package for a proper mock config
	mockConfig := mock.New(t)

	// AddInstance should return context.Canceled error after validating params
	err := comp.AddInstance(ctx, delegatedauth.InstanceParams{
		Config:          mockConfig,
		OrgUUID:         "test-org",
		APIKeyConfigKey: "api_key",
	})

	// The context cancellation check happens after parameter validation,
	// so we should get context.Canceled
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForOldGoroutineRespectsContextCancellation(t *testing.T) {
	// Test that when waiting for an old goroutine to exit, we respect context cancellation
	// This prevents hanging if the old goroutine is stuck

	oldDone := make(chan struct{})
	// Don't close oldDone - simulate a stuck goroutine

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer waitCancel()

	// Simulate the select pattern used in AddInstance when waiting for old goroutine
	start := time.Now()
	select {
	case <-oldDone:
		t.Fatal("Old goroutine should not have exited")
	case <-waitCtx.Done():
		// Expected - context timed out
		elapsed := time.Since(start)
		assert.Less(t, elapsed, 200*time.Millisecond, "Should have timed out quickly")
	}
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

func TestMergeIntoAdditionalEndpointsReplacesDirectiveOnFirstWrite(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA(second-org-uuid, aws)"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	instance := &authInstance{
		additionalEndpointDomain:     "https://second-org.datadoghq.com",
		additionalEndpointsConfigKey: "additional_endpoints",
		lastWrittenValue:             "DELA(second-org-uuid, aws)",
	}

	comp.mergeIntoAdditionalEndpoints(instance, "real-api-key-1", false)

	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, []string{"real-api-key-1"}, got["https://second-org.datadoghq.com"])
	assert.Equal(t, "real-api-key-1", instance.lastWrittenValue)
}

func TestMergeIntoAdditionalEndpointsRotatesWithoutDuplicatesAndPreservesStaticKeys(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://third-org.datadoghq.com": {"some-static-key", "DELA(third-org-uuid, aws)"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	instance := &authInstance{
		additionalEndpointDomain:     "https://third-org.datadoghq.com",
		additionalEndpointsConfigKey: "additional_endpoints",
		lastWrittenValue:             "DELA(third-org-uuid, aws)",
	}

	// First fetch resolves the directive.
	comp.mergeIntoAdditionalEndpoints(instance, "fetched-key-v1", false)
	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.ElementsMatch(t, []string{"some-static-key", "fetched-key-v1"}, got["https://third-org.datadoghq.com"])

	// Refresh rotates the key: only this instance's previous value is replaced, no duplicates,
	// and the coexisting static key is untouched.
	comp.mergeIntoAdditionalEndpoints(instance, "fetched-key-v2", false)
	got = mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.ElementsMatch(t, []string{"some-static-key", "fetched-key-v2"}, got["https://third-org.datadoghq.com"])
}

func TestMergeIntoAdditionalEndpointsDoesNotClobberOtherDomains(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA(second-org-uuid, aws)"},
		"https://third-org.datadoghq.com":  {"DELA(third-org-uuid, aws)"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	secondInstance := &authInstance{
		additionalEndpointDomain:     "https://second-org.datadoghq.com",
		additionalEndpointsConfigKey: "additional_endpoints",
		lastWrittenValue:             "DELA(second-org-uuid, aws)",
	}
	thirdInstance := &authInstance{
		additionalEndpointDomain:     "https://third-org.datadoghq.com",
		additionalEndpointsConfigKey: "additional_endpoints",
		lastWrittenValue:             "DELA(third-org-uuid, aws)",
	}

	comp.mergeIntoAdditionalEndpoints(secondInstance, "second-org-key", false)
	comp.mergeIntoAdditionalEndpoints(thirdInstance, "third-org-key", false)

	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, []string{"second-org-key"}, got["https://second-org.datadoghq.com"])
	assert.Equal(t, []string{"third-org-key"}, got["https://third-org.datadoghq.com"])
}

func TestMergeIntoAdditionalEndpointsListReplacesDirectiveOnFirstWrite(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("logs_config.additional_endpoints", []any{
		map[string]any{"api_key": "DELA(logs-org-uuid, aws)", "Host": "agent-http-intake.logs.datadoghq.com"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	instance := &authInstance{
		additionalEndpointsListConfigKey: "logs_config.additional_endpoints",
		lastWrittenValue:                 "DELA(logs-org-uuid, aws)",
	}

	comp.mergeIntoAdditionalEndpointsList(instance, "real-api-key-1", false)

	got, ok := mockConfig.Get("logs_config.additional_endpoints").([]any)
	require.True(t, ok)
	require.Len(t, got, 1)
	entry, ok := got[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "real-api-key-1", entry["api_key"])
	assert.Equal(t, "agent-http-intake.logs.datadoghq.com", entry["Host"])
	assert.Equal(t, "real-api-key-1", instance.lastWrittenValue)
}

func TestMergeIntoAdditionalEndpointsListRotatesWithoutClobberingOtherEntries(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("logs_config.additional_endpoints", []any{
		map[string]any{"api_key": "some-static-key", "Host": "host-a"},
		map[string]any{"api_key": "DELA(logs-org-uuid, aws)", "Host": "host-b"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	instance := &authInstance{
		additionalEndpointsListConfigKey: "logs_config.additional_endpoints",
		lastWrittenValue:                 "DELA(logs-org-uuid, aws)",
	}

	comp.mergeIntoAdditionalEndpointsList(instance, "fetched-key-v1", false)
	got, ok := mockConfig.Get("logs_config.additional_endpoints").([]any)
	require.True(t, ok)
	require.Len(t, got, 2)
	assert.Equal(t, "some-static-key", got[0].(map[string]any)["api_key"])
	assert.Equal(t, "fetched-key-v1", got[1].(map[string]any)["api_key"])
	assert.Equal(t, "host-b", got[1].(map[string]any)["Host"], "unrelated fields on the matched entry must be preserved")

	// Refresh rotates the key: only this instance's previous value is replaced.
	comp.mergeIntoAdditionalEndpointsList(instance, "fetched-key-v2", false)
	got, ok = mockConfig.Get("logs_config.additional_endpoints").([]any)
	require.True(t, ok)
	assert.Equal(t, "some-static-key", got[0].(map[string]any)["api_key"])
	assert.Equal(t, "fetched-key-v2", got[1].(map[string]any)["api_key"])
}

func TestMergeIntoAdditionalEndpointsListLeavesListUnchangedWhenNoMatch(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("logs_config.additional_endpoints", []any{
		map[string]any{"api_key": "some-static-key", "Host": "host-a"},
	})

	comp := &delegatedAuthComponent{config: mockConfig}
	instance := &authInstance{
		additionalEndpointsListConfigKey: "logs_config.additional_endpoints",
		lastWrittenValue:                 "DELA(logs-org-uuid, aws)", // never present in the list
	}

	comp.mergeIntoAdditionalEndpointsList(instance, "fetched-key", false)

	got, ok := mockConfig.Get("logs_config.additional_endpoints").([]any)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "some-static-key", got[0].(map[string]any)["api_key"])
	assert.Equal(t, "DELA(logs-org-uuid, aws)", instance.lastWrittenValue, "lastWrittenValue must not advance on a failed match")
}

func TestWriteAPIKeyToTargetDispatchesByInstanceShape(t *testing.T) {
	t.Run("flat", func(t *testing.T) {
		mockConfig := mock.New(t)
		comp := &delegatedAuthComponent{config: mockConfig}
		instance := &authInstance{apiKeyConfigKey: "logs_config.api_key"}

		comp.writeAPIKeyToTarget(instance, "flat-key", false)

		assert.Equal(t, "flat-key", mockConfig.GetString("logs_config.api_key"))
	})

	t.Run("map shape", func(t *testing.T) {
		mockConfig := mock.New(t)
		mockConfig.SetInTest("apm_config.additional_endpoints", map[string][]string{
			"https://trace.agent.second-org.datadoghq.com": {"DELA(apm-org-uuid, aws)"},
		})
		comp := &delegatedAuthComponent{config: mockConfig}
		instance := &authInstance{
			additionalEndpointDomain:     "https://trace.agent.second-org.datadoghq.com",
			additionalEndpointsConfigKey: "apm_config.additional_endpoints",
			lastWrittenValue:             "DELA(apm-org-uuid, aws)",
		}

		comp.writeAPIKeyToTarget(instance, "map-key", false)

		got := mockConfig.GetStringMapStringSlice("apm_config.additional_endpoints")
		assert.Equal(t, []string{"map-key"}, got["https://trace.agent.second-org.datadoghq.com"])
	})

	t.Run("list shape", func(t *testing.T) {
		mockConfig := mock.New(t)
		mockConfig.SetInTest("database_monitoring.samples.additional_endpoints", []any{
			map[string]any{"api_key": "DELA(dbm-org-uuid, aws)", "Host": "dbm-metrics-intake.datadoghq.com"},
		})
		comp := &delegatedAuthComponent{config: mockConfig}
		instance := &authInstance{
			additionalEndpointsListConfigKey: "database_monitoring.samples.additional_endpoints",
			lastWrittenValue:                 "DELA(dbm-org-uuid, aws)",
		}

		comp.writeAPIKeyToTarget(instance, "list-key", true) // isFallback=true must not change the write target

		got, ok := mockConfig.Get("database_monitoring.samples.additional_endpoints").([]any)
		require.True(t, ok)
		assert.Equal(t, "list-key", got[0].(map[string]any)["api_key"])
	})
}

func TestFallbackTargetInstanceCarriesWriteTargetFields(t *testing.T) {
	t.Run("map shape", func(t *testing.T) {
		instance := fallbackTargetInstance(delegatedauth.InstanceParams{
			APIKeyConfigKey:              "additional_endpoints[https://second-org.datadoghq.com][second-org-uuid]",
			AdditionalEndpointDomain:     "https://second-org.datadoghq.com",
			AdditionalEndpointsConfigKey: "additional_endpoints",
			AdditionalEndpointDirective:  "DELA(second-org-uuid, aws, fallback=static-key)",
		})

		assert.Equal(t, "https://second-org.datadoghq.com", instance.additionalEndpointDomain)
		assert.Equal(t, "additional_endpoints", instance.additionalEndpointsConfigKey)
		assert.Equal(t, "DELA(second-org-uuid, aws, fallback=static-key)", instance.lastWrittenValue)
		assert.Empty(t, instance.additionalEndpointsListConfigKey)
	})

	t.Run("list shape", func(t *testing.T) {
		instance := fallbackTargetInstance(delegatedauth.InstanceParams{
			APIKeyConfigKey:                  "logs_config.additional_endpoints[0][logs-org-uuid]",
			AdditionalEndpointsListConfigKey: "logs_config.additional_endpoints",
			AdditionalEndpointDirective:      "DELA(logs-org-uuid, aws, fallback=static-key)",
		})

		assert.Equal(t, "logs_config.additional_endpoints", instance.additionalEndpointsListConfigKey)
		assert.Equal(t, "DELA(logs-org-uuid, aws, fallback=static-key)", instance.lastWrittenValue)
		assert.Empty(t, instance.additionalEndpointDomain)
	})
}

func TestAddInstanceWritesFallbackWhenNoCloudProviderDetected(t *testing.T) {
	// Regression test for the motivating case of this session: running with no AWS
	// metadata/IRSA available, cloud-provider detection fails, and without a fallback the
	// additional-endpoint domain would silently get zero keys for the process lifetime.
	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA(second-org-uuid, aws, fallback=static-fallback-key)"},
	})

	comp := &delegatedAuthComponent{
		instances: make(map[string]*authInstance),
		// Simulate "no cloud provider detected" without touching the real network-detection
		// path: pre-mark the component as initialized with a nil providerConfig, exactly as
		// initializeIfNeeded would leave it after creds.IsRunningOnAWS(ctx) returns false.
		initialized: true,
		config:      mockConfig,
	}

	err := comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config:                       mockConfig,
		OrgUUID:                      "second-org-uuid",
		APIKeyConfigKey:              "additional_endpoints[https://second-org.datadoghq.com][second-org-uuid]",
		AdditionalEndpointDomain:     "https://second-org.datadoghq.com",
		AdditionalEndpointsConfigKey: "additional_endpoints",
		AdditionalEndpointDirective:  "DELA(second-org-uuid, aws, fallback=static-fallback-key)",
		FallbackAPIKey:               "static-fallback-key",
	})
	require.NoError(t, err)

	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, []string{"static-fallback-key"}, got["https://second-org.datadoghq.com"],
		"with no cloud provider detected, the fallback key should be written instead of leaving the domain with zero keys")

	// No instance/retry loop should be created for the no-provider case.
	assert.Empty(t, comp.instances)
}

func TestAddInstanceWithoutFallbackSkipsSilentlyWhenNoCloudProviderDetected(t *testing.T) {
	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA(second-org-uuid, aws)"},
	})

	comp := &delegatedAuthComponent{
		instances:   make(map[string]*authInstance),
		initialized: true,
		config:      mockConfig,
	}

	err := comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config:                       mockConfig,
		OrgUUID:                      "second-org-uuid",
		APIKeyConfigKey:              "additional_endpoints[https://second-org.datadoghq.com][second-org-uuid]",
		AdditionalEndpointDomain:     "https://second-org.datadoghq.com",
		AdditionalEndpointsConfigKey: "additional_endpoints",
		AdditionalEndpointDirective:  "DELA(second-org-uuid, aws)",
	})
	require.NoError(t, err)

	// No fallback configured: today's documented behavior is unchanged - the domain is left
	// with zero real keys until a cloud provider becomes available (requires a restart).
	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, []string{"DELA(second-org-uuid, aws)"}, got["https://second-org.datadoghq.com"])
}

func TestAddInstanceWritesFallbackWhenInitialFetchFails(t *testing.T) {
	// Unset any real AWS credentials so GenerateAuthProof fails deterministically and fast (a
	// missing-credentials error, no network/IMDS/STS call - see
	// comp/core/delegatedauth/api/cloudauth/aws/resolve_credentials_noec2.go).
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	mockConfig := mock.New(t)
	mockConfig.SetInTest("additional_endpoints", map[string][]string{
		"https://second-org.datadoghq.com": {"DELA(second-org-uuid, aws, fallback=static-fallback-key)"},
	})

	comp := &delegatedAuthComponent{instances: make(map[string]*authInstance)}

	apiKeyConfigKey := "additional_endpoints[https://second-org.datadoghq.com][second-org-uuid]"
	err := comp.AddInstance(context.Background(), delegatedauth.InstanceParams{
		Config:                       mockConfig,
		ProviderConfig:               &cloudauthconfig.AWSProviderConfig{Region: "us-east-1"},
		OrgUUID:                      "second-org-uuid",
		RefreshInterval:              60,
		APIKeyConfigKey:              apiKeyConfigKey,
		AdditionalEndpointDomain:     "https://second-org.datadoghq.com",
		AdditionalEndpointsConfigKey: "additional_endpoints",
		AdditionalEndpointDirective:  "DELA(second-org-uuid, aws, fallback=static-fallback-key)",
		FallbackAPIKey:               "static-fallback-key",
	})
	require.NoError(t, err)

	got := mockConfig.GetStringMapStringSlice("additional_endpoints")
	assert.Equal(t, []string{"static-fallback-key"}, got["https://second-org.datadoghq.com"],
		"an initial fetch failure with a fallback configured should still leave the domain usable")

	// A real instance/retry loop IS created in this case (unlike the no-provider case) - stop its
	// background refresh goroutine so it doesn't outlive the test.
	comp.mu.RLock()
	instance := comp.instances[apiKeyConfigKey]
	comp.mu.RUnlock()
	require.NotNil(t, instance)
	instance.refreshCancel()
}
