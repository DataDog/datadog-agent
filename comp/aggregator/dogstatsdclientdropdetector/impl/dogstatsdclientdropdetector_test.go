// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdclientdropdetectorimpl

import (
	"context"
	"errors"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/require"

	dogstatsdclientdropdetector "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclientdropdetector/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dogstatsdclientdrops "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dogstatsdclientdrops"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	hostuuid "github.com/DataDog/datadog-agent/pkg/util/uuid"
)

const testHostname = "test-node"

type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(hook compdef.Hook) {
	l.hooks = append(l.hooks, hook)
}

func (l *testLifecycle) start(t testing.TB) {
	t.Helper()
	for _, hook := range l.hooks {
		if hook.OnStart != nil {
			require.NoError(t, hook.OnStart(context.Background()))
		}
	}
}

func newTestComponent(t testing.TB) (*component, *healthplatformmock.Mock) {
	t.Helper()
	healthPlatform := healthplatformmock.New(t)
	return newTestComponentWithHealthPlatform(t, healthPlatform, testHostname), healthPlatform
}

func newTestComponentWithHealthPlatform(t testing.TB, healthPlatform healthplatformstore.Component, hostnameValue string) *component {
	t.Helper()
	hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostnameValue))
	lifecycle := &testLifecycle{}
	provides := NewComponent(Requires{
		Lifecycle: lifecycle, Config: config.NewMock(t), Log: logmock.New(t),
		Hostname: hostname, HealthPlatform: healthPlatform,
	})
	lifecycle.start(t)
	return provides.Comp.(*component)
}

// persistedOnlyHealthPlatform models the real store immediately after restart:
// lifecycle metadata is available, but the full active issue payload is not.
type persistedOnlyHealthPlatform struct {
	*healthplatformmock.Mock
	activeByName map[string][]string
}

type reportErrorHealthPlatform struct {
	*healthplatformmock.Mock
	issueID   string
	reportErr error
}

func (p *reportErrorHealthPlatform) ReportIssue(issue *healthplatformpayload.Issue) error {
	if issue.Id == p.issueID && p.reportErr != nil {
		return p.reportErr
	}
	return p.Mock.ReportIssue(issue)
}

func (p *persistedOnlyHealthPlatform) GetActiveIssueIDsByIssueName(issueName string) []string {
	return append([]string(nil), p.activeByName[issueName]...)
}

func useTestClock(detector *component) func(time.Duration) {
	now := time.Unix(1_700_000_000, 0)
	detector.now = func() time.Time { return now }
	return func(elapsed time.Duration) { now = now.Add(elapsed) }
}

func completeWindow(detector *component, stats clientByteStats) {
	observeClientStats(detector, dogstatsdclientdrops.ClientLibraryGo, stats)
	detector.CompleteFinalDogStatsDSerieFlush()
}

func observeClientStats(detector *component, library dogstatsdclientdrops.ClientLibrary, stats clientByteStats) {
	for _, observation := range []struct {
		metric dogstatsdclientdropdetector.ClientByteMetric
		bytes  float64
	}{
		{metric: dogstatsdclientdropdetector.ClientByteMetricSent, bytes: stats.sent},
		{metric: dogstatsdclientdropdetector.ClientByteMetricDropped, bytes: stats.dropped},
		{metric: dogstatsdclientdropdetector.ClientByteMetricDroppedQueue, bytes: stats.droppedQueue},
		{metric: dogstatsdclientdropdetector.ClientByteMetricDroppedWriter, bytes: stats.droppedWriter},
	} {
		if observation.bytes > 0 {
			detector.ObserveClientBytes(string(library), observation.metric, observation.bytes)
		}
	}
}

func goClientState(detector *component) *clientState {
	return detector.clientState(dogstatsdclientdrops.ClientLibraryGo)
}

func TestDroppedRatioThreshold(t *testing.T) {
	ratio, violated := droppedRatio(clientByteStats{sent: 990, dropped: 10})
	require.Equal(t, 0.01, ratio)
	require.False(t, violated)

	ratio, violated = droppedRatio(clientByteStats{sent: 980, dropped: 20})
	require.Equal(t, 0.02, ratio)
	require.True(t, violated)

	ratio, violated = droppedRatio(clientByteStats{droppedQueue: 10, droppedWriter: 10})
	require.Zero(t, ratio)
	require.False(t, violated)
}

func TestDropReasonBreakdown(t *testing.T) {
	unclassified, complete := (clientByteStats{dropped: 20, droppedQueue: 12, droppedWriter: 8}).dropReasonBreakdown()
	require.Zero(t, unclassified)
	require.True(t, complete)

	unclassified, complete = (clientByteStats{dropped: 20, droppedQueue: 12}).dropReasonBreakdown()
	require.Equal(t, 8.0, unclassified)
	require.False(t, complete)
}

func TestComponentIgnoresWindowBeforeStartupReconciliation(t *testing.T) {
	healthPlatform := healthplatformmock.New(t)
	hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname(testHostname))
	lifecycle := &testLifecycle{}
	detector := NewComponent(Requires{
		Lifecycle: lifecycle, Config: config.NewMock(t), Log: logmock.New(t),
		Hostname: hostname, HealthPlatform: healthPlatform,
	}).Comp.(*component)

	completeWindow(detector, clientByteStats{sent: 98, dropped: 2})
	state := goClientState(detector)
	require.Equal(t, clientByteStats{}, state.stats)
	require.False(t, state.confirmationPending)
	require.Nil(t, healthPlatform.GetIssue(state.issueID))

	lifecycle.start(t)
	completeWindow(detector, clientByteStats{sent: 98, dropped: 2})
	require.True(t, state.confirmationPending)
}

func TestComponentReportsAndResolvesUDSDropIssue(t *testing.T) {
	detector, healthPlatform := newTestComponent(t)
	require.Equal(t, time.Minute, detector.unhealthyConfirmationDuration)
	require.Equal(t, 30*time.Minute, detector.recoveryConfirmationDuration)
	advance := useTestClock(detector)
	unhealthy := clientByteStats{sent: 980, dropped: 20, droppedQueue: 12, droppedWriter: 8}
	completeWindow(detector, unhealthy)

	issueID := goClientState(detector).issueID
	require.Nil(t, healthPlatform.GetIssue(issueID))

	advance(detector.unhealthyConfirmationDuration)
	completeWindow(detector, unhealthy)

	issue := healthPlatform.GetIssue(issueID)
	require.NotNil(t, issue)
	require.Equal(t, dogstatsdclientdrops.UDSIssueName(dogstatsdclientdrops.ClientLibraryGo), issue.IssueName)
	require.Contains(t, issue.Title, testHostname)
	require.Contains(t, issue.Title, "UDS")
	require.Equal(t, "uds", issue.Extra.GetFields()["transport_family"].GetStringValue())
	require.True(t, issue.Extra.GetFields()["detection_evidence_available"].GetBoolValue())
	require.Equal(t, 0.02, issue.Extra.GetFields()["dropped_ratio"].GetNumberValue())
	require.Equal(t, 24.0, issue.Extra.GetFields()["bytes_dropped_queue"].GetNumberValue())
	require.Equal(t, 16.0, issue.Extra.GetFields()["bytes_dropped_writer"].GetNumberValue())
	require.True(t, issue.Extra.GetFields()["drop_reason_breakdown_complete"].GetBoolValue())
	require.Zero(t, issue.Extra.GetFields()["bytes_dropped_unclassified"].GetNumberValue())
	firstDescription := issue.Description

	completeWindow(detector, clientByteStats{sent: 970, dropped: 30})
	require.Equal(t, firstDescription, healthPlatform.GetIssue(issueID).Description)

	// Drop-reason telemetry alone is not proof that the primary ratio recovered.
	completeWindow(detector, clientByteStats{droppedQueue: 1})
	require.NotNil(t, healthPlatform.GetIssue(issueID))
	require.Empty(t, healthPlatform.ResolvedIDs())

	completeWindow(detector, clientByteStats{sent: 1000})
	advance(detector.recoveryConfirmationDuration)
	completeWindow(detector, clientByteStats{sent: 1000})

	require.Nil(t, healthPlatform.GetIssue(issueID))
	require.Equal(t, []string{issueID}, healthPlatform.ResolvedIDs())
}

func TestComponentMaintainsIndependentIssuesPerClientLibrary(t *testing.T) {
	detector, healthPlatform := newTestComponent(t)
	advance := useTestClock(detector)
	goState := detector.clientState(dogstatsdclientdrops.ClientLibraryGo)
	pythonState := detector.clientState(dogstatsdclientdrops.ClientLibraryPython)

	for range 2 {
		observeClientStats(detector, dogstatsdclientdrops.ClientLibraryGo, clientByteStats{sent: 98, dropped: 2})
		observeClientStats(detector, dogstatsdclientdrops.ClientLibraryPython, clientByteStats{sent: 100})
		detector.CompleteFinalDogStatsDSerieFlush()
		advance(detector.unhealthyConfirmationDuration)
	}

	goIssue := healthPlatform.GetIssue(goState.issueID)
	require.NotNil(t, goIssue)
	require.Equal(t, dogstatsdclientdrops.UDSIssueName(dogstatsdclientdrops.ClientLibraryGo), goIssue.IssueName)
	require.Nil(t, healthPlatform.GetIssue(pythonState.issueID))
}

func TestComponentPendingTransitionsRequireContinuousEvidence(t *testing.T) {
	detector, healthPlatform := newTestComponent(t)
	advance := useTestClock(detector)
	issueID := goClientState(detector).issueID
	observe := func(sent, dropped float64) {
		completeWindow(detector, clientByteStats{sent: sent, dropped: dropped})
	}

	observe(98, 2)
	advance(detector.unhealthyConfirmationDuration)
	observe(100, 0)
	require.Nil(t, healthPlatform.GetIssue(issueID), "a healthy window must cancel a pending open")

	observe(98, 2)
	advance(detector.unhealthyConfirmationDuration)
	detector.CompleteFinalDogStatsDSerieFlush()
	observe(98, 2)
	require.Nil(t, healthPlatform.GetIssue(issueID), "missing telemetry must cancel a pending open")

	advance(detector.unhealthyConfirmationDuration)
	observe(98, 2)
	require.NotNil(t, healthPlatform.GetIssue(issueID))

	observe(100, 0)
	advance(detector.recoveryConfirmationDuration)
	observe(98, 2)
	require.NotNil(t, healthPlatform.GetIssue(issueID), "an unhealthy window must cancel a pending resolution")

	observe(100, 0)
	advance(detector.recoveryConfirmationDuration)
	detector.CompleteFinalDogStatsDSerieFlush()
	observe(100, 0)
	require.NotNil(t, healthPlatform.GetIssue(issueID), "missing telemetry must cancel a pending resolution")

	advance(detector.recoveryConfirmationDuration)
	observe(100, 0)
	require.Nil(t, healthPlatform.GetIssue(issueID))
}

func TestComponentRetriesFailedIssueReport(t *testing.T) {
	issueID := dogstatsdclientdrops.UDSIssueIDForHost(dogstatsdclientdrops.ClientLibraryGo, hostuuid.GetUUID(), testHostname)
	healthPlatform := &reportErrorHealthPlatform{
		Mock:      healthplatformmock.New(t),
		issueID:   issueID,
		reportErr: errors.New("report failed"),
	}
	detector := newTestComponentWithHealthPlatform(t, healthPlatform, testHostname)
	advance := useTestClock(detector)
	unhealthy := clientByteStats{sent: 98, dropped: 2}

	completeWindow(detector, unhealthy)
	advance(detector.unhealthyConfirmationDuration)
	completeWindow(detector, unhealthy)
	state := goClientState(detector)
	require.False(t, state.issueActive)
	require.True(t, state.confirmationPending)

	healthPlatform.reportErr = nil
	completeWindow(detector, unhealthy)
	require.True(t, state.issueActive)
	require.False(t, state.confirmationPending)
}

func TestComponentReconcilesPersistedIssueState(t *testing.T) {
	t.Run("rehydrates persisted-only issue before receiving telemetry", func(t *testing.T) {
		issueID := dogstatsdclientdrops.UDSIssueIDForHost(dogstatsdclientdrops.ClientLibraryGo, hostuuid.GetUUID(), testHostname)
		baseStore := healthplatformmock.New(t)
		healthPlatform := &persistedOnlyHealthPlatform{
			Mock:         baseStore,
			activeByName: map[string][]string{dogstatsdclientdrops.UDSIssueName(dogstatsdclientdrops.ClientLibraryGo): {issueID}},
		}

		detector := newTestComponentWithHealthPlatform(t, healthPlatform, testHostname)
		restoredIssue := baseStore.GetIssue(issueID)
		require.NotNil(t, restoredIssue)
		require.Contains(t, restoredIssue.Description, "awaiting current client telemetry")
		require.False(t, restoredIssue.Extra.GetFields()["detection_evidence_available"].GetBoolValue())
		state := goClientState(detector)
		require.True(t, state.issueActive)
		require.True(t, state.issueNeedsRefresh)

		advance := useTestClock(detector)
		completeWindow(detector, clientByteStats{sent: 100})
		require.NotNil(t, baseStore.GetIssue(issueID), "one healthy window must not resolve a restored issue")
		advance(detector.recoveryConfirmationDuration)
		completeWindow(detector, clientByteStats{sent: 100})

		require.Nil(t, baseStore.GetIssue(issueID))
		require.Equal(t, []string{issueID}, baseStore.ResolvedIDs())
	})

	t.Run("does not resolve absent issue after restart", func(t *testing.T) {
		detector, healthPlatform := newTestComponent(t)
		advance := useTestClock(detector)

		completeWindow(detector, clientByteStats{sent: 100})
		advance(detector.recoveryConfirmationDuration)
		completeWindow(detector, clientByteStats{sent: 100})

		require.Empty(t, healthPlatform.ResolvedIDs())
	})
}
