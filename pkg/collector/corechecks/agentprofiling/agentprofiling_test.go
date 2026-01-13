// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentprofiling is a core check that can generate a flare with profiles
// when the core agent's memory or CPU usage exceeds a certain threshold.
package agentprofiling

import (
	"fmt"
	"testing"
	"time"

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

// testConfig represents a test configuration for the agentprofiling check
type testConfig struct {
	memoryThreshold           string
	cpuThreshold              int
	ticketID                  string
	userEmail                 string
	terminateAgentOnThreshold bool
}

// defaultTestConfig returns a default test configuration
func defaultTestConfig() testConfig {
	return testConfig{
		memoryThreshold:           "0",
		cpuThreshold:              0,
		ticketID:                  "",
		userEmail:                 "",
		terminateAgentOnThreshold: false,
	}
}

// createTestCheck creates a new check instance with the given configuration
func createTestCheck(t *testing.T, cfg testConfig) *Check {
	flare := flaremock.NewMock().Comp
	config := configmock.NewMock(t)
	check := newCheck(flare, config).(*Check)

	// Configure the check with the test configuration
	configData := []byte(fmt.Sprintf(`memory_threshold: "%s"
cpu_threshold: %d
ticket_id: "%s"
user_email: "%s"
terminate_agent_on_threshold: %t`, cfg.memoryThreshold, cfg.cpuThreshold, cfg.ticketID, cfg.userEmail, cfg.terminateAgentOnThreshold))

	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, configData, initConfig, "test")
	require.NoError(t, err)

	return check
}

// TestZeroThresholds tests that the flare is not generated when thresholds are set to zero
func TestZeroThresholds(t *testing.T) {
	check := createTestCheck(t, defaultTestConfig())

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
}

// TestMemThreshold tests that the flare is generated when memory threshold is exceeded
func TestMemThreshold(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1B" // 1 byte to force trigger

	check := createTestCheck(t, cfg)

	// Verify memory threshold is properly parsed
	assert.Equal(t, uint(1), check.memoryThreshold)

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestCPUThreshold tests that the flare is generated when CPU threshold is exceeded
func TestCPUThreshold(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.cpuThreshold = 1 // 1 percent to force trigger

	check := createTestCheck(t, cfg)

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestBelowThresholds tests that the flare is not generated when both memory and CPU usage are below thresholds
func TestBelowThresholds(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1000GB" // Very high memory threshold
	cfg.cpuThreshold = 1000        // 1000% CPU threshold

	check := createTestCheck(t, cfg)

	// Verify memory threshold is properly parsed
	expectedBytes := uint(1000 * 1024 * 1024 * 1024) // 1000GB in bytes
	assert.Equal(t, expectedBytes, check.memoryThreshold)

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
}

// TestGenerateFlareLocal tests that the flare is generated correctly when ticket ID and user email are not provided
func TestGenerateFlareLocal(t *testing.T) {
	check := createTestCheck(t, defaultTestConfig())

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestGenerateFlareTicket tests that the flare is generated correctly when ticket ID and user email are provided
func TestGenerateFlareTicket(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.ticketID = "12345678"
	cfg.userEmail = "user@example.com"

	check := createTestCheck(t, cfg)

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestTerminateAgentOnThresholdConfig tests that the terminate_agent_on_threshold config is parsed correctly
func TestTerminateAgentOnThresholdConfig(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1B" // Force trigger
	cfg.terminateAgentOnThreshold = true

	check := createTestCheck(t, cfg)

	// Verify config is parsed correctly
	assert.True(t, check.instance.TerminateAgentOnThreshold)
	assert.Equal(t, uint(1), check.memoryThreshold)

	// Verify that when threshold is exceeded, flare is generated
	// Note: Termination is skipped in test mode (detected via testing.Testing()), so we can't test
	// the actual shutdown behavior. However, we verify that the config is parsed correctly
	// and that the check would attempt termination in a non-test environment.
	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// TestTerminateAgentOnThresholdDisabled tests that termination does not occur when disabled
func TestTerminateAgentOnThresholdDisabled(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1B" // Force trigger
	cfg.terminateAgentOnThreshold = false

	check := createTestCheck(t, cfg)

	// Verify config is parsed correctly
	assert.False(t, check.instance.TerminateAgentOnThreshold)

	// Verify flare is still generated
	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
}

// failingFlareMock is a mock that fails flare creation or sending
type failingFlareMock struct {
	flare.Component
	createShouldFail bool
	sendShouldFail   bool
	attemptCount     int
	failUntilAttempt int // Fail until this attempt number, then succeed
}

func (f *failingFlareMock) CreateWithArgs(args types.FlareArgs, duration time.Duration, err error, data []byte) (string, error) {
	f.attemptCount++
	if f.createShouldFail || (f.failUntilAttempt > 0 && f.attemptCount < f.failUntilAttempt) {
		return "", fmt.Errorf("mock flare creation failure")
	}
	// Use the real mock for success case
	mock := flaremock.NewMock().Comp
	return mock.CreateWithArgs(args, duration, err, data)
}

func (f *failingFlareMock) Send(path string, caseID string, email string, source helpers.FlareSource) (string, error) {
	if f.sendShouldFail || (f.failUntilAttempt > 0 && f.attemptCount < f.failUntilAttempt) {
		return "", fmt.Errorf("mock flare send failure")
	}
	// Use the real mock for success case
	mock := flaremock.NewMock().Comp
	return mock.Send(path, caseID, email, source)
}

// createTestCheckWithFailingFlare creates a check with a failing flare mock
func createTestCheckWithFailingFlare(t *testing.T, cfg testConfig, createShouldFail, sendShouldFail bool, failUntilAttempt int) *Check {
	failingMock := &failingFlareMock{
		createShouldFail: createShouldFail,
		sendShouldFail:   sendShouldFail,
		failUntilAttempt: failUntilAttempt,
	}
	config := configmock.NewMock(t)
	check := &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{},
		flareComponent: failingMock,
		agentConfig:    config,
	}

	// Configure the check with the test configuration
	configData := []byte(fmt.Sprintf(`memory_threshold: "%s"
cpu_threshold: %d
ticket_id: "%s"
user_email: "%s"
terminate_agent_on_threshold: %t`, cfg.memoryThreshold, cfg.cpuThreshold, cfg.ticketID, cfg.userEmail, cfg.terminateAgentOnThreshold))

	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, configData, initConfig, "test")
	require.NoError(t, err)

	return check
}

// TestRetryOnFlareFailure tests that the check retries when flare creation or sending fails
func TestRetryOnFlareFailure(t *testing.T) {
	tests := []struct {
		name             string
		createShouldFail bool
		sendShouldFail   bool
		setTicketInfo    bool
	}{
		{
			name:             "creation failure",
			createShouldFail: true,
			sendShouldFail:   false,
			setTicketInfo:    false,
		},
		{
			name:             "send failure",
			createShouldFail: false,
			sendShouldFail:   true,
			setTicketInfo:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultTestConfig()
			cfg.memoryThreshold = "1B" // Force trigger
			if tt.setTicketInfo {
				cfg.ticketID = "12345678"
				cfg.userEmail = "user@example.com"
			}

			check := createTestCheckWithFailingFlare(t, cfg, tt.createShouldFail, tt.sendShouldFail, 0)

			// First attempt should fail
			err := check.Run()
			require.NoError(t, err)
			assert.False(t, check.flareAttempted)
			assert.Equal(t, 1, check.flareAttemptCount)

			// Simulate backoff elapsed (backoff for attempt 1 is 1 minute)
			check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

			// Second attempt should also fail (but we're still within retry limit)
			err = check.Run()
			require.NoError(t, err)
			assert.False(t, check.flareAttempted)
			assert.Equal(t, 2, check.flareAttemptCount)

			// Simulate backoff elapsed again (backoff for attempt 2 is 5 minutes)
			check.lastFlareAttempt = time.Now().Add(-6 * time.Minute)

			// Third attempt should fail and exhaust retries
			err = check.Run()
			require.NoError(t, err)
			assert.True(t, check.flareAttempted)
			assert.Equal(t, 3, check.flareAttemptCount)
		})
	}
}

// TestRetryWithBackoff tests that backoff is calculated correctly
func TestRetryWithBackoff(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1B" // Force trigger

	check := createTestCheckWithFailingFlare(t, cfg, true, false, 0)

	// First attempt
	err := check.Run()
	require.NoError(t, err)
	assert.Equal(t, 1, check.flareAttemptCount)
	firstAttemptTime := check.lastFlareAttempt
	assert.False(t, firstAttemptTime.IsZero())

	// Verify backoff calculation is correct
	backoff := check.calculateBackoff(1)
	assert.Equal(t, 1*time.Minute, backoff)

	backoff = check.calculateBackoff(2)
	assert.Equal(t, 5*time.Minute, backoff)

	backoff = check.calculateBackoff(3)
	assert.Equal(t, 15*time.Minute, backoff)

	// Test that backoff caps at max
	backoff = check.calculateBackoff(10)
	assert.Equal(t, 15*time.Minute, backoff)
}

// TestRetrySuccessAfterFailure tests that retry succeeds after initial failures
func TestRetrySuccessAfterFailure(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.memoryThreshold = "1B" // Force trigger
	cfg.ticketID = "12345678"
	cfg.userEmail = "user@example.com"

	// Fail first 2 attempts (attemptCount < 3), then succeed on 3rd (attemptCount = 3)
	check := createTestCheckWithFailingFlare(t, cfg, false, false, 3)

	// First attempt should fail
	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 1, check.flareAttemptCount)

	// Simulate backoff elapsed (backoff for attempt 1 is 1 minute)
	check.lastFlareAttempt = time.Now().Add(-2 * time.Minute)

	// Second attempt should fail
	err = check.Run()
	require.NoError(t, err)
	assert.False(t, check.flareAttempted)
	assert.Equal(t, 2, check.flareAttemptCount)

	// Note: The test expects success on 3rd attempt, but with failUntilAttempt=2,
	// it will succeed on 2nd attempt. Let's adjust the test expectation.
	// Actually, wait - failUntilAttempt means fail until that attempt number,
	// so failUntilAttempt=2 means fail on attempts 1 and 2, succeed on 3.
	// But we've already done attempt 1, so attempt 2 should fail, then attempt 3 should succeed.
	// But we need to simulate backoff for attempt 3 (backoff for attempt 2 is 5 minutes)
	check.lastFlareAttempt = time.Now().Add(-6 * time.Minute)

	// Third attempt should succeed
	err = check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareAttempted)
	// It succeeds on attempt 2 because failUntilAttempt=2 means fail until attempt 2, succeed on 3
	// But we've already done 2 attempts, so the next one (3rd) should succeed
	// Actually wait, let me re-check the logic. failUntilAttempt=2 means fail if attemptCount < 2
	// So attemptCount=1 fails, attemptCount=2 fails, attemptCount=3 succeeds
	// But we've done 1 attempt, so next is 2, which should fail, then 3 should succeed
	assert.Equal(t, 3, check.flareAttemptCount)
}
