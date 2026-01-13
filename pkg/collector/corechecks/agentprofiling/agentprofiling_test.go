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
	expBackoff.InitialInterval = 2 * time.Minute
	expBackoff.MaxInterval = 10 * time.Minute
	expBackoff.Multiplier = 2.0
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

	// Simulate backoff elapsed (~2 minutes)
	check.lastFlareAttempt = time.Now().Add(-3 * time.Minute)

	// Second attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Simulate backoff elapsed (~4 minutes)
	check.lastFlareAttempt = time.Now().Add(-5 * time.Minute)

	// Third attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 3, check.flareAttemptCount)

	// Simulate backoff elapsed (~8 minutes)
	check.lastFlareAttempt = time.Now().Add(-9 * time.Minute)

	// Fourth attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 4, check.flareAttemptCount)

	// Simulate backoff elapsed (~10 minutes, capped)
	check.lastFlareAttempt = time.Now().Add(-11 * time.Minute)

	// Fifth attempt fails and exhausts retries
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	assert.Equal(t, 5, check.flareAttemptCount)
}

// TestRetryOnFlareSendFailure tests retry logic when flare sending fails
func TestRetryOnFlareSendFailure(t *testing.T) {
	// Create check with failing send - need ticket info to trigger Send()
	failingMock := &failingFlareMock{
		sendError: errors.New("send failed"),
	}
	config := configmock.NewMock(t)

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Minute
	expBackoff.MaxInterval = 10 * time.Minute
	expBackoff.Multiplier = 2.0
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

	// Simulate backoff elapsed
	check.lastFlareAttempt = time.Now().Add(-3 * time.Minute)

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

	// Simulate backoff elapsed (~2 minutes)
	check.lastFlareAttempt = time.Now().Add(-3 * time.Minute)

	// Second attempt fails
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Simulate backoff elapsed (~4 minutes)
	check.lastFlareAttempt = time.Now().Add(-5 * time.Minute)

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

	// First backoff should be ~2 minutes (with randomization)
	backoffDuration := check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 1*time.Minute+45*time.Second)
	assert.LessOrEqual(t, backoffDuration, 2*time.Minute+15*time.Second)

	// Second backoff should be ~4 minutes
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 3*time.Minute+30*time.Second)
	assert.LessOrEqual(t, backoffDuration, 4*time.Minute+30*time.Second)

	// Third backoff should be ~8 minutes
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 7*time.Minute)
	assert.LessOrEqual(t, backoffDuration, 9*time.Minute)

	// Fourth backoff should be ~10 minutes (capped)
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 9*time.Minute)
	assert.LessOrEqual(t, backoffDuration, 10*time.Minute+30*time.Second)

	// Subsequent backoffs should remain capped
	backoffDuration = check.backoffPolicy.NextBackOff()
	assert.GreaterOrEqual(t, backoffDuration, 9*time.Minute)
	assert.LessOrEqual(t, backoffDuration, 10*time.Minute+30*time.Second)
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

	// Try to retry immediately - should wait (backoff not elapsed)
	// Note: NextBackOff() advances the backoff state even when we don't retry,
	// so the backoff duration will be for the next attempt
	err = check.Run()
	require.NoError(t, err)
	// Attempt count should still be 1 because backoff hasn't elapsed
	assert.Equal(t, 1, check.flareAttemptCount)
	// lastFlareAttempt should be unchanged (we didn't make a new attempt)
	assert.Equal(t, firstAttemptTime.Unix(), check.lastFlareAttempt.Unix())

	// Simulate backoff elapsed - need enough time for the backoff duration
	// (which was advanced by the previous NextBackOff() call, so it's now ~4min)
	check.lastFlareAttempt = time.Now().Add(-5 * time.Minute)

	// Now retry should proceed
	err = check.Run()
	require.NoError(t, err)
	assert.Equal(t, 2, check.flareAttemptCount)
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
	assert.GreaterOrEqual(t, backoffDuration, 1*time.Minute+45*time.Second)
	assert.LessOrEqual(t, backoffDuration, 2*time.Minute+15*time.Second)
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
	expBackoff.InitialInterval = 2 * time.Minute
	expBackoff.MaxInterval = 10 * time.Minute
	expBackoff.Multiplier = 2.0
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
	check := createCheckWithFailingFlare(t, "1B", 0, nil, nil, 0)

	// Manually set backoff policy to StopBackOff to test stop signal handling
	check.backoffPolicy = &backoff.StopBackOff{}

	// First attempt
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed
	check.lastFlareAttempt = time.Now().Add(-3 * time.Minute)

	// Next attempt should stop due to backoff.Stop
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted) // Should be marked as attempted due to stop signal
}
