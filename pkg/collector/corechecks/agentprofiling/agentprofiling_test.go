// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofiling

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	flaremock "github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// createTestCheck creates a check instance with the given configuration
func createTestCheck(t *testing.T, memoryThreshold string, cpuThreshold int, ticketID, userEmail string, terminateAgent bool) *Check {
	flareComp := flaremock.NewMock().Comp
	config := configmock.NewMock(t)
	check := newCheck(flareComp, config).(*Check)

	configData := []byte(fmt.Sprintf(`memory_threshold: "%s"
cpu_threshold: %d
ticket_id: "%s"
user_email: "%s"
terminate_agent_on_threshold: %t`, memoryThreshold, cpuThreshold, ticketID, userEmail, terminateAgent))

	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, configData, initConfig, "test")
	require.NoError(t, err)

	return check
}

// failingFlareMock is a mock flare component that can fail on creation or sending
type failingFlareMock struct {
	flare.Component
	createError error
	sendError   error
	callCount   int
	failUntil   int // Fail until this call count, then succeed
}

func (m *failingFlareMock) CreateWithArgs(args types.FlareArgs, duration time.Duration, err error, data []byte) (string, error) {
	m.callCount++
	if m.createError != nil || (m.failUntil > 0 && m.callCount < m.failUntil) {
		return "", errors.New("mock flare creation failure")
	}
	mock := flaremock.NewMock().Comp
	return mock.CreateWithArgs(args, duration, err, data)
}

func (m *failingFlareMock) Send(path string, caseID string, email string, source helpers.FlareSource) (string, error) {
	if m.sendError != nil || (m.failUntil > 0 && m.callCount < m.failUntil) {
		return "", errors.New("mock flare send failure")
	}
	mock := flaremock.NewMock().Comp
	return mock.Send(path, caseID, email, source)
}

// newTestBackoffPolicy creates a backoff policy with standard test settings
func newTestBackoffPolicy() *backoff.ExponentialBackOff {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Minute
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.Multiplier = 5.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.Reset()
	return expBackoff
}

// createCheckWithFailingFlare creates a check with a failing flare mock
func createCheckWithFailingFlare(t *testing.T, memoryThreshold string, cpuThreshold int, createError, sendError error, failUntil int) *Check {
	failingMock := &failingFlareMock{
		createError: createError,
		sendError:   sendError,
		failUntil:   failUntil,
	}
	config := configmock.NewMock(t)

	check := &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{},
		backoffPolicy:  newTestBackoffPolicy(),
		flareComponent: failingMock,
		agentConfig:    config,
	}

	configData := []byte(fmt.Sprintf(`memory_threshold: "%s"
cpu_threshold: %d`, memoryThreshold, cpuThreshold))

	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, configData, initConfig, "test")
	require.NoError(t, err)

	return check
}

// TestConfigurationParsing tests that configuration is parsed correctly
func TestConfigurationParsing(t *testing.T) {
	check := createTestCheck(t, "100MB", 50, "TICKET-123", "user@example.com", true)

	assert.Equal(t, uint(100*1024*1024), check.memoryThreshold)
	assert.Equal(t, 50, check.instance.CPUThreshold)
	assert.Equal(t, "TICKET-123", check.instance.TicketID)
	assert.Equal(t, "user@example.com", check.instance.UserEmail)
	assert.True(t, check.instance.TerminateAgentOnThreshold)
}

// TestZeroThresholds tests that the check skips when both thresholds are zero
func TestZeroThresholds(t *testing.T) {
	check := createTestCheck(t, "0", 0, "", "", false)

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 0, check.flareAttemptCount)
}

// TestThresholdExceeded tests flare generation when thresholds are exceeded
func TestThresholdExceeded(t *testing.T) {
	tests := []struct {
		name            string
		memoryThreshold string
		cpuThreshold    int
		description     string
	}{
		{
			name:            "memory threshold exceeded",
			memoryThreshold: "1B", // 1 byte threshold will always be exceeded
			cpuThreshold:    0,
			description:     "memory threshold",
		},
		{
			name:            "CPU threshold exceeded",
			memoryThreshold: "0",
			cpuThreshold:    1, // 1% CPU threshold
			description:     "CPU threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := createTestCheck(t, tt.memoryThreshold, tt.cpuThreshold, "", "", false)

			err := check.Run()
			require.NoError(t, err)
			assert.True(t, check.flareAttempted, "Flare should be attempted when %s is exceeded", tt.description)
			assert.Equal(t, 1, check.flareAttemptCount)
		})
	}
}

// TestBelowThresholds tests that flare is not generated when usage is below thresholds
func TestBelowThresholds(t *testing.T) {
	check := createTestCheck(t, "1000GB", 1000, "", "", false) // Very high thresholds

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 0, check.flareAttemptCount)
}

// TestFlareGeneration tests flare generation with and without ticket info
func TestFlareGeneration(t *testing.T) {
	tests := []struct {
		name      string
		ticketID  string
		userEmail string
	}{
		{
			name:      "local generation without ticket",
			ticketID:  "",
			userEmail: "",
		},
		{
			name:      "generation with ticket",
			ticketID:  "TICKET-123",
			userEmail: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := createTestCheck(t, "1B", 0, tt.ticketID, tt.userEmail, false)

			err := check.generateFlare()
			require.NoError(t, err)
			assert.True(t, check.flareAttempted)
		})
	}
}

// TestRetryOnFlareCreationFailure tests retry logic when flare creation fails
func TestRetryOnFlareCreationFailure(t *testing.T) {
	check := createCheckWithFailingFlare(t, "1B", 0, errors.New("creation failed"), nil, 0)

	// First attempt fails
	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed (~1 minute)
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

	// Second attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Simulate backoff elapsed (~5 minutes)
	check.lastFlareAttempt = time.Now().Add(-6 * time.Minute)

	// Third attempt fails and exhausts retries
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 3, check.flareAttemptCount)
}

// TestRetryOnFlareSendFailure tests retry logic when flare sending fails
func TestRetryOnFlareSendFailure(t *testing.T) {
	// Create check with failing send - need ticket info to trigger Send()
	failingMock := &failingFlareMock{
		sendError: errors.New("send failed"),
	}
	config := configmock.NewMock(t)

	check := &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{TicketID: "TICKET-123", UserEmail: "user@example.com"},
		backoffPolicy:  newTestBackoffPolicy(),
		flareComponent: failingMock,
		agentConfig:    config,
	}

	configData := []byte(`memory_threshold: "1B"
cpu_threshold: 0
ticket_id: "1234567"
user_email: "user@example.com"`)

	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, configData, initConfig, "test")
	require.NoError(t, err)

	// First attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed (~1 minute)
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

	// Second attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)
}

// TestRetrySuccessAfterFailure tests that retry succeeds after initial failures
func TestRetrySuccessAfterFailure(t *testing.T) {
	// Fail first 2 attempts, succeed on 3rd
	check := createCheckWithFailingFlare(t, "1B", 0, nil, nil, 3)

	// First attempt fails
	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed (~1 minute)
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

	// Second attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Simulate backoff elapsed (~5 minutes)
	check.lastFlareAttempt = time.Now().Add(-6 * time.Minute)

	// Third attempt succeeds
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 3, check.flareAttemptCount)
}

// TestBackoffWait tests that retry waits for backoff duration
func TestBackoffWait(t *testing.T) {
	check := createCheckWithFailingFlare(t, "1B", 0, errors.New("failed"), nil, 0)

	// First attempt fails
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)
	firstAttemptTime := check.lastFlareAttempt
	assert.False(t, firstAttemptTime.IsZero())
	// Backoff duration not yet calculated (will be calculated on next Run when checking if we need to wait)
	assert.Equal(t, time.Duration(0), check.nextBackoffDuration)

	// Try to retry immediately - should wait (backoff not elapsed)
	// This Run() call will calculate the backoff duration (~1 minute) and check if we need to wait
	err = check.Run()
	require.NoError(t, err)
	// Attempt count should still be 1 because backoff hasn't elapsed
	assert.Equal(t, 1, check.flareAttemptCount)
	// lastFlareAttempt should be unchanged (we didn't make a new attempt)
	assert.Equal(t, firstAttemptTime.Unix(), check.lastFlareAttempt.Unix())
	// Backoff duration should now be cached (~1 minute) and remain the same on subsequent checks
	assert.Greater(t, check.nextBackoffDuration, 50*time.Second)
	assert.LessOrEqual(t, check.nextBackoffDuration, 1*time.Minute+10*time.Second)

	// Call Run() again - backoff duration should remain the same (not advance)
	err = check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)
	assert.Greater(t, check.nextBackoffDuration, 50*time.Second)
	assert.LessOrEqual(t, check.nextBackoffDuration, 1*time.Minute+10*time.Second)

	// Simulate backoff elapsed - need enough time for the backoff duration (~1 minute)
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

	// Now retry should proceed
	err = check.Run()
	require.NoError(t, err)
	assert.Equal(t, 2, check.flareAttemptCount)
	// After second attempt fails, next backoff will be calculated on next Run() call
	// Clear the cached duration to simulate what happens after backoff elapsed
	check.nextBackoffDuration = 0

	// Next Run() call will calculate the next backoff duration (~5 minutes)
	err = check.Run()
	require.NoError(t, err)
	assert.Equal(t, 2, check.flareAttemptCount) // Still 2, waiting for backoff
	assert.Greater(t, check.nextBackoffDuration, 4*time.Minute+30*time.Second)
	assert.LessOrEqual(t, check.nextBackoffDuration, 5*time.Minute+30*time.Second)
}

// TestBackoffResetOnFirstAttempt tests that backoff is reset on first threshold detection
func TestBackoffResetOnFirstAttempt(t *testing.T) {
	check := createCheckWithFailingFlare(t, "1B", 0, nil, nil, 0)

	// Advance backoff policy state before first attempt
	check.backoffPolicy.NextBackOff()
	check.backoffPolicy.NextBackOff()
	advancedBackoff := check.backoffPolicy.NextBackOff()
	assert.Greater(t, advancedBackoff, 1*time.Minute+10*time.Second, "Advanced backoff should be > 1min")

	// First attempt should reset backoff policy
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)

	// After reset, next backoff should be initial interval (~1min), not the advanced value
	backoffDuration := check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 50*time.Second)
	assert.LessOrEqual(t, backoffDuration, 1*time.Minute+10*time.Second, "Backoff should be reset to initial interval")
}

// TestNoRetryAfterSuccess tests that no retries occur after successful flare generation
func TestNoRetryAfterSuccess(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "", "", false)

	// First attempt succeeds
	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Subsequent runs should not attempt again
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount) // Still 1, no new attempts
}

// TestTerminateAgentConfig tests that terminate_agent_on_threshold config is parsed correctly
func TestTerminateAgentConfig(t *testing.T) {
	tests := []struct {
		name              string
		terminateAgent    bool
		expectedTerminate bool
	}{
		{
			name:              "terminate enabled",
			terminateAgent:    true,
			expectedTerminate: true,
		},
		{
			name:              "terminate disabled",
			terminateAgent:    false,
			expectedTerminate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := createTestCheck(t, "1B", 0, "", "", tt.terminateAgent)

			assert.Equal(t, tt.expectedTerminate, check.instance.TerminateAgentOnThreshold)

			// Run check - termination is skipped in test mode, but flare should be generated
			err := check.Run()
			require.NoError(t, err)
			assert.True(t, check.flareAttempted)
		})
	}
}

// TestFlareComponentNil tests behavior when flare component is nil
func TestFlareComponentNil(t *testing.T) {
	config := configmock.NewMock(t)

	check := &Check{
		CheckBase:       core.NewCheckBase(CheckName),
		instance:        &Config{MemoryThreshold: "1B"},
		backoffPolicy:   newTestBackoffPolicy(),
		flareComponent:  nil,
		agentConfig:     config,
		memoryThreshold: 1,
	}

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted) // Marked as attempted even though skipped
}

// TestBackoffPolicyStop tests handling of backoff.Stop signal
func TestBackoffPolicyStop(t *testing.T) {
	// First attempt must fail to trigger retry logic and backoff.Stop handling
	check := createCheckWithFailingFlare(t, "1B", 0, errors.New("creation failed"), nil, 0)

	// Manually set backoff policy to StopBackOff to test stop signal handling
	check.backoffPolicy = &backoff.StopBackOff{}

	// First attempt fails
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)
	assert.False(t, check.flareAttempted) // Should not be marked as attempted yet

	// Simulate backoff elapsed - this will trigger NextBackOff() which returns Stop
	check.lastFlareAttempt = time.Now().Add(-3 * time.Minute)
	check.nextBackoffDuration = 0 // Clear cached duration to trigger recalculation

	// Next attempt should stop due to backoff.Stop
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted) // Should be marked as attempted due to stop signal
}
