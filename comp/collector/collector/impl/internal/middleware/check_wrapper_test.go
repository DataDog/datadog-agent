// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package middleware

import (
	"context"
	"sync"
	"testing"
	"time"

	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCheck struct {
	check.Check
	runCalled bool
	cancelled bool
	id        checkid.ID
}

func (m *mockCheck) Run() error {
	m.runCalled = true
	return nil
}

func (m *mockCheck) Cancel() {
	m.cancelled = true
}

func (m *mockCheck) ID() checkid.ID {
	if m.id == "" {
		return checkid.ID("mock_check")
	}
	return m.id
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

type mockIssueReporter struct {
	healthplatformstore.Component
}

type issueAwareCheck struct {
	mockCheck
	reporter healthplatformstore.Component
}

func (c *issueAwareCheck) SetIssueReporter(r healthplatformstore.Component) {
	c.reporter = r
}

func TestCheckWrapperInjectsIssueReporter(t *testing.T) {
	reporter := &mockIssueReporter{}
	inner := &issueAwareCheck{}

	wrapper := NewCheckWrapper(
		inner,
		nil,
		option.None[agenttelemetry.Component](),
		option.New[healthplatformstore.Component](reporter),
	)

	require.NotNil(t, wrapper)
	assert.Equal(t, reporter, inner.reporter, "reporter should be injected at construction")
}

func TestCheckWrapperInjectsIssueReporterThroughShadowCheck(t *testing.T) {
	reporter := &mockIssueReporter{}
	inner := &issueAwareCheck{}
	shadow := check.NewShadowCheck(inner, 0)

	wrapper := NewCheckWrapper(
		shadow,
		nil,
		option.None[agenttelemetry.Component](),
		option.New[healthplatformstore.Component](reporter),
	)

	require.NotNil(t, wrapper)
	assert.Equal(t, reporter, inner.reporter, "reporter should be injected through the shadow wrapper")
}

func TestCheckWrapperSkipsNonIssueAwareCheck(t *testing.T) {
	reporter := &mockIssueReporter{}
	inner := &mockCheck{}

	wrapper := NewCheckWrapper(
		inner,
		nil,
		option.None[agenttelemetry.Component](),
		option.New[healthplatformstore.Component](reporter),
	)

	require.NotNil(t, wrapper)
	err := wrapper.Run()
	require.NoError(t, err)
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
		option.None[healthplatformstore.Component](),
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

func TestCheckWrapperPreservesShadowIdentity(t *testing.T) {
	inner := &mockCheck{}
	shadow := check.NewShadowCheck(inner, 0)

	wrapper := NewCheckWrapper(
		shadow,
		nil,
		option.None[agenttelemetry.Component](),
		option.None[healthplatformstore.Component](),
	)

	assert.True(t, check.IsShadow(wrapper))
	assert.Same(t, shadow, wrapper.Unwrap())
}

func TestCheckWrapperUsesShadowSenderManagerForShadowCleanup(t *testing.T) {
	normalSenderManager := newRecordingSenderManager()
	shadowSenderManager := newRecordingSenderManager()
	inner := &mockCheck{id: checkid.ID("cpu:abc123")}
	shadow := check.NewShadowCheckWithSenderManagerOverride(inner, 0, shadowSenderManager)

	wrapper := NewCheckWrapper(
		shadow,
		normalSenderManager,
		option.None[agenttelemetry.Component](),
		option.None[healthplatformstore.Component](),
	)

	wrapper.Cancel()

	shadowSenderManager.requireDestroyed(t, checkid.ID("cpu:abc123:shadow"))
	assert.True(t, inner.cancelled)
	assert.Empty(t, normalSenderManager.destroyedIDs)
}

func TestCheckWrapperUsesNormalSenderManagerWhenThereIsNoOverride(t *testing.T) {
	normalSenderManager := newRecordingSenderManager()
	inner := &mockCheck{id: checkid.ID("cpu:abc123")}

	wrapper := NewCheckWrapper(
		inner,
		normalSenderManager,
		option.None[agenttelemetry.Component](),
		option.None[healthplatformstore.Component](),
	)

	wrapper.Cancel()

	normalSenderManager.requireDestroyed(t, checkid.ID("cpu:abc123"))
	assert.True(t, inner.cancelled)
}

type recordingSenderManager struct {
	mu           sync.Mutex
	destroyedIDs []checkid.ID
	destroyedCh  chan checkid.ID
}

func newRecordingSenderManager() *recordingSenderManager {
	return &recordingSenderManager{destroyedCh: make(chan checkid.ID, 1)}
}

func (m *recordingSenderManager) GetSender(checkid.ID) (sender.Sender, error) {
	return nil, nil
}

func (m *recordingSenderManager) SetSender(sender.Sender, checkid.ID) error {
	return nil
}

func (m *recordingSenderManager) DestroySender(id checkid.ID) {
	m.mu.Lock()
	m.destroyedIDs = append(m.destroyedIDs, id)
	m.mu.Unlock()
	m.destroyedCh <- id
}

func (m *recordingSenderManager) GetDefaultSender() (sender.Sender, error) {
	return nil, nil
}

func (m *recordingSenderManager) requireDestroyed(t *testing.T, expectedID checkid.ID) {
	t.Helper()

	select {
	case gotID := <-m.destroyedCh:
		assert.Equal(t, expectedID, gotID)
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for sender cleanup of %s", expectedID)
	}
}
