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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	flaremock "github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
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
	assert.False(t, check.flareGenerated)
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
	assert.True(t, check.flareGenerated)
}

// TestCPUThreshold tests that the flare is generated when CPU threshold is exceeded
func TestCPUThreshold(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.cpuThreshold = 1 // 1 percent to force trigger

	check := createTestCheck(t, cfg)

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareGenerated)
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
	assert.False(t, check.flareGenerated)
}

// TestGenerateFlareLocal tests that the flare is generated correctly when ticket ID and user email are not provided
func TestGenerateFlareLocal(t *testing.T) {
	check := createTestCheck(t, defaultTestConfig())

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareGenerated)
}

// TestGenerateFlareTicket tests that the flare is generated correctly when ticket ID and user email are provided
func TestGenerateFlareTicket(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.ticketID = "12345678"
	cfg.userEmail = "user@example.com"

	check := createTestCheck(t, cfg)

	err := check.generateFlare()
	require.NoError(t, err)
	assert.True(t, check.flareGenerated)
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
	// Note: Termination is skipped in test mode (detected via os.Args), so we can't test
	// the actual shutdown behavior. However, we verify that the config is parsed correctly
	// and that the check would attempt termination in a non-test environment.
	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.flareGenerated)
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
	assert.True(t, check.flareGenerated)
}
