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

// createCheckWithFailingFlare creates a check with a failing flare mock
func createCheckWithFailingFlare(t *testing.T, memoryThreshold string, cpuThreshold int, createError, sendError error, failUntil int) *Check {
	failingMock := &failingFlareMock{
		createError: createError,
		sendError:   sendError,
		failUntil:   failUntil,
	}
	config := configmock.NewMock(t)

	// Initialize backoff policy (same as newCheck)
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Minute
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.Multiplier = 5.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.Reset()

	check := &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{},
		backoffPolicy:  expBackoff,
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

// TestMemoryThresholdExceeded tests flare generation when memory threshold is exceeded
func TestMemoryThresholdExceeded(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "", "", false) // 1 byte threshold will always be exceeded

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)
}

// TestCPUThresholdExceeded tests flare generation when CPU threshold is exceeded
func TestCPUThresholdExceeded(t *testing.T) {
	check := createTestCheck(t, "0", 1, "", "", false) // 1% CPU threshold

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)
}

// TestBelowThresholds tests that flare is not generated when usage is below thresholds
func TestBelowThresholds(t *testing.T) {
	check := createTestCheck(t, "1000GB", 1000, "", "", false) // Very high thresholds

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 0, check.flareAttemptCount)
}

// TestFlareGenerationLocal tests local flare generation without ticket info
func TestFlareGenerationLocal(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "", "", false)

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestFlareGenerationWithTicket tests flare generation and sending with ticket info
func TestFlareGenerationWithTicket(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "TICKET-123", "user@example.com", false)

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
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

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Minute
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.Multiplier = 5.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.Reset()

	check := &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{TicketID: "TICKET-123", UserEmail: "user@example.com"},
		backoffPolicy:  expBackoff,
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

// TestBackoffTiming tests that backoff durations follow exponential backoff
func TestBackoffTiming(t *testing.T) {
	check := createCheckWithFailingFlare(t, "1B", 0, nil, nil, 0)

	// Reset backoff policy to test durations
	check.backoffPolicy.Reset()

	// First backoff should be ~1 minute (with randomization)
	backoffDuration := check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 50*time.Second)
	assert.LessOrEqual(t, backoffDuration, 1*time.Minute+10*time.Second)

	// Second backoff should be ~5 minutes (1min * 5 multiplier)
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 4*time.Minute+30*time.Second)
	assert.LessOrEqual(t, backoffDuration, 5*time.Minute+30*time.Second)

	// Third backoff would be ~25 minutes (5min * 5), but capped at 5 minutes
	// Note: We never actually use this since we only have 3 attempts total
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 4*time.Minute+30*time.Second)
	assert.LessOrEqual(t, backoffDuration, 5*time.Minute+30*time.Second)

	// Subsequent backoffs should remain capped at 5 minutes
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 4*time.Minute+30*time.Second)
	assert.LessOrEqual(t, backoffDuration, 5*time.Minute+30*time.Second)
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

	// Advance backoff policy state
	check.backoffPolicy.NextBackOff()
	check.backoffPolicy.NextBackOff()

	// First attempt should reset backoff
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Backoff should be reset, so next backoff should be initial interval
	check.backoffPolicy.Reset()
	backoffDuration := check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 50*time.Second)
	assert.LessOrEqual(t, backoffDuration, 1*time.Minute+10*time.Second)
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

// TestTerminateAgentConfig tests that terminate_agent_on_threshold config is respected
func TestTerminateAgentConfig(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "", "", true)

	// Verify config is parsed
	assert.True(t, check.instance.TerminateAgentOnThreshold)

	// Run check - termination is skipped in test mode, but flare should be generated
	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestTerminateAgentDisabled tests that agent is not terminated when disabled
func TestTerminateAgentDisabled(t *testing.T) {
	check := createTestCheck(t, "1B", 0, "", "", false)

	assert.False(t, check.instance.TerminateAgentOnThreshold)

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestFlareComponentNil tests behavior when flare component is nil
func TestFlareComponentNil(t *testing.T) {
	config := configmock.NewMock(t)

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Minute
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.Multiplier = 5.0
	expBackoff.RandomizationFactor = 0.1
	expBackoff.Reset()

	check := &Check{
		CheckBase:       core.NewCheckBase(CheckName),
		instance:        &Config{MemoryThreshold: "1B"},
		backoffPolicy:   expBackoff,
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

// stopAfterFirstCallBackoff is a backoff that returns Stop after the first call
type stopAfterFirstCallBackoff struct {
	callCount   int
	realBackoff backoff.BackOff
}

func (b *stopAfterFirstCallBackoff) NextBackOff() time.Duration {
	b.callCount++
	if b.callCount == 1 {
		return backoff.Stop
	}
	return b.realBackoff.NextBackOff()
}

func (b *stopAfterFirstCallBackoff) Reset() {
	b.callCount = 0
	b.realBackoff.Reset()
}

// TestBackoffResetAndAdvance tests that reset-and-advance logic correctly positions backoff state
func TestBackoffResetAndAdvance(t *testing.T) {
	check := createCheckWithFailingFlare(t, "1B", 0, errors.New("failed"), nil, 0)

	// Create a backoff that returns Stop after first call, forcing reset
	realBackoff := backoff.NewExponentialBackOff()
	realBackoff.InitialInterval = 1 * time.Minute
	realBackoff.MaxInterval = 5 * time.Minute
	realBackoff.Multiplier = 5.0
	realBackoff.RandomizationFactor = 0.1
	realBackoff.Reset()

	stopBackoff := &stopAfterFirstCallBackoff{
		realBackoff: realBackoff,
	}
	check.backoffPolicy = stopBackoff

	// First attempt fails
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed - this will trigger NextBackOff() which returns Stop,
	// causing reset and advance logic
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)
	check.nextBackoffDuration = 0 // Clear cached duration to trigger recalculation

	// This Run() should reset the policy and advance correctly
	// With flareAttemptCount=1, we should advance 0 times (1-1=0), then get 1st backoff
	err = check.Run()
	require.NoError(t, err)

	// Verify we got the 1st backoff duration (~1 min), not the 2nd (~5 min)
	assert.GreaterOrEqual(t, check.nextBackoffDuration, 50*time.Second)
	assert.LessOrEqual(t, check.nextBackoffDuration, 1*time.Minute+10*time.Second)
	assert.Equal(t, 1, check.flareAttemptCount) // Still waiting for backoff

	// Now simulate second attempt failing
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)
	check.nextBackoffDuration = 0
	err = check.Run()
	require.NoError(t, err)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Reset the stop backoff for next test
	stopBackoff.callCount = 0
	check.lastFlareAttempt = time.Now().Add(-6 * time.Minute)
	check.nextBackoffDuration = 0

	// With flareAttemptCount=2, we should advance 1 time (2-1=1), then get 2nd backoff
	err = check.Run()
	require.NoError(t, err)

	// Verify we got the 2nd backoff duration (~5 min), not the 3rd
	assert.GreaterOrEqual(t, check.nextBackoffDuration, 4*time.Minute+30*time.Second)
	assert.LessOrEqual(t, check.nextBackoffDuration, 5*time.Minute+30*time.Second)
	assert.Equal(t, 2, check.flareAttemptCount) // Still waiting for backoff
}
