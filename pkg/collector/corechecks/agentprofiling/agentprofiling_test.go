// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentprofiling is a core check that can capture a memory profile of the
// core agent when the core agent's memory usage exceeds a certain threshold.

package agentprofiling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	flaremock "github.com/DataDog/datadog-agent/comp/core/flare/flareimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func createTestCheck(t *testing.T) *AgentProfilingCheck {
	flare := flaremock.NewMock().Comp
	cfg := configmock.NewMock(t)
	return newCheck(flare, cfg).(*AgentProfilingCheck)
}

// TestConfigParse tests that the configuration is parsed correctly
func TestConfigParse(t *testing.T) {
	check := createTestCheck(t)
	config := []byte(`
memory_threshold: "10MB"
cpu_threshold: 30
ticket_id: "12345678"
user_email: "test@datadoghq.com"
`)
	initConfig := []byte("")
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := check.Configure(senderManager, integration.FakeConfigHash, config, initConfig, "test")
	require.NoError(t, err)
	assert.Equal(t, "10MB", check.instance.MemoryThreshold)
	assert.Equal(t, 30, check.instance.CPUThreshold)
	assert.Equal(t, "12345678", check.instance.TicketID)
	assert.Equal(t, "test@datadoghq.com", check.instance.UserEmail)
}

// TestZeroThresholds tests that the flare is not generated when thresholds are set to zero
func TestZeroThresholds(t *testing.T) {
	check := createTestCheck(t)
	check.instance.MemoryThreshold = "0"
	check.instance.CPUThreshold = 0

	err := check.Run()
	require.NoError(t, err)
	assert.False(t, check.profileCaptured)
}

// TestMemThreshold tests that the flare is generated when memory threshold is exceeded
func TestMemThreshold(t *testing.T) {
	check := createTestCheck(t)
	check.instance.MemoryThreshold = "1B" // 1 byte to force trigger

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.profileCaptured)
}

// TestCPUThreshold tests that the flare is generated when CPU threshold is exceeded
func TestCPUThreshold(t *testing.T) {
	check := createTestCheck(t)
	check.instance.CPUThreshold = 1 // 1 byte to force trigger

	err := check.Run()
	require.NoError(t, err)
	assert.True(t, check.profileCaptured)
}

// TestGenerateFlareLocal tests that the flare is generated correctly when ticket ID and user email are not provided
func TestGenerateFlareLocal(t *testing.T) {
	check := createTestCheck(t)
	check.instance.TicketID = ""
	check.instance.UserEmail = ""

	err := check.generateFlare()
	require.NoError(t, err)

	assert.True(t, check.profileCaptured)
}

// TestGenerateFlareZendesk tests that the flare is generated correctly when ticket ID and user email are provided
func TestGenerateFlareZendesk(t *testing.T) {
	check := createTestCheck(t)
	check.instance.TicketID = "12345678"
	check.instance.UserEmail = "user@example.com"

	err := check.generateFlare()
	require.NoError(t, err)

	assert.True(t, check.profileCaptured)
}
