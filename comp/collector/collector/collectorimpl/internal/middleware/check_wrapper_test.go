// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package middleware

import (
	"context"
	"testing"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCheck struct {
	check.Check
	runCalled bool
}

func (m *mockCheck) Run() error {
	m.runCalled = true
	return nil
}

func (m *mockCheck) String() string {
	return "mock_check"
}

type mockTelemetry struct {
	agenttelemetry.Component
	spanStarted bool
	spanName    string
}

func (m *mockTelemetry) StartStartupSpan(name string) (*installertelemetry.Span, context.Context) {
	m.spanStarted = true
	m.spanName = name
	return &installertelemetry.Span{}, context.Background()
}

func (m *mockTelemetry) SendEvent(_ string, _ []byte) error {
	return nil
}

func TestCheckWrapperCreatesSpan(t *testing.T) {
	// Create a mock check
	mockCheck := &mockCheck{}

	// Create a mock telemetry component
	mockTelemetry := &mockTelemetry{}

	// Create the check wrapper with the mock telemetry
	wrapper := NewCheckWrapper(
		mockCheck,
		nil, // senderManager is not needed for this test
		option.New[agenttelemetry.Component](mockTelemetry),
	)

	// Run the check
	err := wrapper.Run()

	// Verify the check was run
	require.True(t, mockCheck.runCalled)
	require.NoError(t, err)

	// Verify a span was started
	assert.True(t, mockTelemetry.spanStarted)
	assert.Equal(t, "check.mock_check", mockTelemetry.spanName)
}
