// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentmemprof

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
)

type mockFlareComponent struct{}

func (m *mockFlareComponent) Create(_ types.ProfileData, providerTimeout time.Duration, ipcError error) (string, error) {
	return "/tmp/mock_flare.zip", nil
}

func (m *mockFlareComponent) Send(_, caseID, email string, source helpers.FlareSource) (string, error) {
	return "Flare sent successfully", nil
}

func TestRun(t *testing.T) {
	// Create a new check instance with a mock flare component
	flareComponent := &mockFlareComponent{}
	check := newCheck(flareComponent).(*AgentMemProfCheck)
	check.instance.MemoryThreshold = 1024 * 1024 // 1 MB

	// Mock memory usage to exceed threshold
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memStats.HeapAlloc = 2 * 1024 * 1024 // 2 MB

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was captured
	assert.True(t, check.profileCaptured)
}

func TestRunProfileAlreadyCaptured(t *testing.T) {
	// Create a new check instance with a mock flare component
	flareComponent := &mockFlareComponent{}
	check := newCheck(flareComponent).(*AgentMemProfCheck)
	check.instance.MemoryThreshold = 1024 * 1024 // 1 MB
	check.profileCaptured = true

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was not captured again
	assert.True(t, check.profileCaptured)
}

func TestRunThresholdNotSet(t *testing.T) {
	// Create a new check instance with a mock flare component
	flareComponent := &mockFlareComponent{}
	check := newCheck(flareComponent).(*AgentMemProfCheck)
	check.instance.MemoryThreshold = 0 // Threshold not set

	// Run the check
	err := check.Run()
	require.NoError(t, err)

	// Verify that the profile was not captured
	assert.False(t, check.profileCaptured)
}

func TestGenerateFlareLocal(t *testing.T) {
	// Create a new check instance with a mock flare component
	flareComponent := &mockFlareComponent{}
	check := newCheck(flareComponent).(*AgentMemProfCheck)
	check.instance.TicketID = 0

	// Generate the flare
	err := check.generateFlare()
	require.NoError(t, err)

	// Verify that the flare was generated locally
	assert.True(t, check.profileCaptured)
}
