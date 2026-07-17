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
	configcomponent "github.com/DataDog/datadog-agent/comp/core/config"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"

	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	installertelemetry "github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingCheck struct {
	mockCheck
	err error
}

func (c *failingCheck) Run() error {
	c.runCalled = true
	return c.err
}

func TestCheckWrapperReportsExecutionFailureAfterThreshold(t *testing.T) {
	cfg := configcomponent.NewMock(t)
	cfg.SetInTest("health_platform.check_execution_failure.enabled", true)
	cfg.SetInTest("health_platform.check_execution_failure.consecutive_failures", 2)
	cfg.SetInTest("health_platform.check_execution_failure.consecutive_successes", 1)

	reporter := storemock.New(t)
	inner := &failingCheck{mockCheck: mockCheck{id: checkid.ID("mysql:abc")}, err: assert.AnError}

	wrapper := NewCheckWrapper(
		inner,
		nil,
		option.None[agenttelemetry.Component](),
		option.New[healthplatformstore.Component](reporter),
		cfg,
	)

	// First failure: below threshold, no issue yet.
	require.Error(t, wrapper.Run())
	count, _ := reporter.GetAllIssues()
	assert.Zero(t, count, "issue must not be reported before the consecutive-failure threshold is crossed")

	// Second failure: threshold crossed, issue reported.
	require.Error(t, wrapper.Run())
	count, issues := reporter.GetAllIssues()
	require.Equal(t, 1, count)
	for _, issue := range issues {
		assert.Equal(t, "Check Execution Failure", issue.GetIssueName())
	}

	// Recovery: consecutive_successes is 1, so a single success resolves the issue.
	inner.err = nil
	require.NoError(t, wrapper.Run())
	count, _ = reporter.GetAllIssues()
	assert.Zero(t, count, "issue must be resolved after the consecutive-success threshold is crossed")
	assert.Len(t, reporter.ResolvedIDs(), 1)
}

func TestCheckWrapperSkipsExecutionFailureReportingWhenDisabled(t *testing.T) {
	cfg := configcomponent.NewMock(t)
	cfg.SetInTest("health_platform.check_execution_failure.enabled", false)

	reporter := storemock.New(t)
	inner := &failingCheck{mockCheck: mockCheck{}, err: assert.AnError}

	wrapper := NewCheckWrapper(
		inner,
		nil,
		option.None[agenttelemetry.Component](),
		option.New[healthplatformstore.Component](reporter),
		cfg,
	)

	for i := 0; i < 5; i++ {
		require.Error(t, wrapper.Run())
	}
	count, _ := reporter.GetAllIssues()
	assert.Zero(t, count, "no issue should be reported when the feature flag is disabled")
}

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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
		configcomponent.NewMock(t),
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
