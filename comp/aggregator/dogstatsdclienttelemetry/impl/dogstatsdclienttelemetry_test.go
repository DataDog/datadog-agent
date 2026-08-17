// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package dogstatsdclienttelemetryimpl

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	dogstatsdclientdrops "github.com/DataDog/datadog-agent/comp/healthplatform/issues/dogstatsdclientdrops"
	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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

func newTestComponent(t testing.TB, tlm telemetry.Component) (Provides, *healthplatformmock.Mock) {
	healthPlatform := healthplatformmock.New(t)
	return newTestComponentWithHealthPlatform(t, tlm, healthPlatform, testHostname), healthPlatform
}

func newTestComponentWithHealthPlatform(t testing.TB, tlm telemetry.Component, healthPlatform healthplatformstore.Component, hostnameValue string) Provides {
	hostname, _ := hostnamemock.NewMock(hostnamemock.MockHostname(hostnameValue))
	lifecycle := &testLifecycle{}
	provides := NewComponent(Requires{
		Lifecycle:      lifecycle,
		Telemetry:      tlm,
		Config:         config.NewMock(t),
		Log:            logmock.New(t),
		Hostname:       hostname,
		HealthPlatform: healthPlatform,
	})
	lifecycle.start(t)
	return provides
}

// persistedOnlyHealthPlatform models the real store immediately after restart:
// lifecycle metadata is available, but the full active issue payload is not.
type persistedOnlyHealthPlatform struct {
	*healthplatformmock.Mock
	activeByName map[string][]string
}

func (p *persistedOnlyHealthPlatform) GetActiveIssueIDsByIssueName(issueName string) []string {
	return append([]string(nil), p.activeByName[issueName]...)
}

func useTestClock(detector *droppedMetricsDetector) func(time.Duration) {
	now := time.Unix(1_700_000_000, 0)
	detector.now = func() time.Time { return now }
	return func(elapsed time.Duration) { now = now.Add(elapsed) }
}

func transportTags(transport string) tagset.CompositeTags {
	return tagset.CompositeTagsFromSlice([]string{"client_transport:" + transport})
}

func completeUDSWindow(component *component, stats clientByteStats) {
	for _, metric := range []struct {
		name  string
		value float64
	}{
		{name: dogStatsDClientBytesSentMetric, value: stats.sent},
		{name: dogStatsDClientBytesDroppedMetric, value: stats.dropped},
		{name: dogStatsDClientBytesDroppedQueueMetric, value: stats.droppedQueue},
		{name: dogStatsDClientBytesDroppedWriterMetric, value: stats.droppedWriter},
	} {
		if metric.value == 0 {
			continue
		}
		component.ObserveFinalDogStatsDSerie(&metrics.Serie{
			Name: metric.name, Tags: transportTags("uds"), MType: metrics.APIRateType, Interval: 1, Points: []metrics.Point{{Value: metric.value}},
		})
	}
	component.CompleteFinalDogStatsDSerieFlush()
}

func TestComponentObservesClientByteRateSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, _ := newTestComponent(t, telemetry)

	for _, test := range []struct {
		name     string
		value    float64
		expected float64
	}{
		{name: "datadog.dogstatsd.client.bytes_sent", value: 0.7, expected: 7},
		{name: "datadog.dogstatsd.client.bytes_dropped", value: 0.3, expected: 3},
		{name: "datadog.dogstatsd.client.bytes_dropped_queue", value: 0.5, expected: 5},
		{name: "datadog.dogstatsd.client.bytes_dropped_writer", value: 0.2, expected: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
				Name:     test.name,
				MType:    metrics.APIRateType,
				Interval: 10,
				Points:   []metrics.Point{{Value: test.value}},
				Tags:     tagset.CompositeTagsFromSlice([]string{"client:go", "client_transport:uds"}),
			})

			metrics, err := telemetry.GetCountMetric("dogstatsd_client", test.name[len("datadog.dogstatsd.client."):])
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			require.Equal(t, map[string]string{"client": "go", "client_transport": "uds"}, metrics[0].Tags())
			require.Equal(t, test.expected, metrics[0].Value())
		})
	}
}

func TestComponentSumsRatePointsInFinalSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, _ := newTestComponent(t, telemetry)

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     dogStatsDClientBytesSentMetric,
		MType:    metrics.APIRateType,
		Interval: 10,
		Points: []metrics.Point{
			{Value: 0.7},
			{Value: 0.3},
		},
	})

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 10.0, metrics[0].Value())
}

func TestComponentPreservesFractionalRecoveredByteTotal(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, _ := newTestComponent(t, telemetry)

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     dogStatsDClientBytesSentMetric,
		MType:    metrics.APIRateType,
		Interval: 10,
		Points:   []metrics.Point{{Value: 0.75}},
	})

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 7.5, metrics[0].Value())
}

func TestComponentIgnoresUnsupportedOrInvalidSeries(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, _ := newTestComponent(t, telemetry)

	provides.Observer.ObserveFinalDogStatsDSerie(&metrics.Serie{
		Name:     "datadog.dogstatsd.client.bytes_sent",
		MType:    metrics.APIRateType,
		Interval: 10,
		Points:   []metrics.Point{{Value: 1}},
	})

	for _, serie := range []*metrics.Serie{
		{
			Name:     "datadog.dogstatsd.client.metrics",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIGaugeType,
			Interval: 10,
			Points:   []metrics.Point{{Value: 7}},
		},
		{
			Name:     "datadog.dogstatsd.client.bytes_sent",
			MType:    metrics.APIRateType,
			Interval: 10,
			Points: []metrics.Point{
				{Value: -0.7},
				{Value: math.NaN()},
				{Value: math.Inf(1)},
				{Value: math.Ldexp(1, 64) / 10},
				{Value: 1e20},
			},
		},
	} {
		provides.Observer.ObserveFinalDogStatsDSerie(serie)
	}

	metrics, err := telemetry.GetCountMetric("dogstatsd_client", "bytes_sent")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	require.Equal(t, 10.0, metrics[0].Value())
}

func TestComponentSharesValidUDSClientBytesWithDetector(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, _ := newTestComponent(t, telemetry)
	component := provides.Observer.(*component)
	udsTags := transportTags("uds")

	for _, serie := range []*metrics.Serie{
		{Name: dogStatsDClientBytesSentMetric, Tags: udsTags, MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 5}}},
		{Name: dogStatsDClientBytesSentMetric, Tags: transportTags("uds-stream"), MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 4}}},
		{Name: dogStatsDClientBytesDroppedMetric, Tags: udsTags, MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 1}}},
		{Name: dogStatsDClientBytesDroppedQueueMetric, Tags: udsTags, MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 0.6}}},
		{Name: dogStatsDClientBytesDroppedWriterMetric, Tags: udsTags, MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 0.4}}},
		{Name: dogStatsDClientBytesSentMetric, Tags: transportTags("udp"), MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 100}}},
		{Name: "customer.metric", MType: metrics.APIRateType, Interval: 10, Points: []metrics.Point{{Value: 100}}},
	} {
		provides.Observer.ObserveFinalDogStatsDSerie(serie)
	}

	stats := component.detector.takeWindow()
	require.Equal(t, clientByteStats{sent: 90, dropped: 10, droppedQueue: 6, droppedWriter: 4}, stats)
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

func TestComponentReportsAndResolvesUDSDropIssue(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, healthPlatform := newTestComponent(t, telemetry)
	component := provides.Observer.(*component)
	require.Equal(t, time.Minute, component.detector.unhealthyConfirmationDuration)
	require.Equal(t, 30*time.Minute, component.detector.recoveryConfirmationDuration)
	advance := useTestClock(&component.detector)
	unhealthy := clientByteStats{sent: 980, dropped: 20, droppedQueue: 12, droppedWriter: 8}
	completeUDSWindow(component, unhealthy)

	issueID := dogstatsdclientdrops.UDSIssueIDForHostname(testHostname)
	require.Nil(t, healthPlatform.GetIssue(issueID))

	advance(component.detector.unhealthyConfirmationDuration)
	completeUDSWindow(component, unhealthy)

	issue := healthPlatform.GetIssue(issueID)
	require.NotNil(t, issue)
	require.Equal(t, dogstatsdclientdrops.UDSIssueName, issue.IssueName)
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

	// A continuing violation stays as one active issue rather than rewriting
	// persistent Agent Health state on every serializer flush.
	completeUDSWindow(component, clientByteStats{sent: 970, dropped: 30})
	require.Equal(t, firstDescription, healthPlatform.GetIssue(issueID).Description)

	// Drop-reason telemetry alone is not proof that the primary ratio recovered.
	completeUDSWindow(component, clientByteStats{droppedQueue: 1})
	require.NotNil(t, healthPlatform.GetIssue(issueID))
	require.Empty(t, healthPlatform.ResolvedIDs())

	completeUDSWindow(component, clientByteStats{sent: 1000})
	advance(component.detector.recoveryConfirmationDuration)
	completeUDSWindow(component, clientByteStats{sent: 1000})

	require.Nil(t, healthPlatform.GetIssue(issueID))
	require.Equal(t, []string{issueID}, healthPlatform.ResolvedIDs())
}

func TestComponentPendingTransitionsRequireContinuousEvidence(t *testing.T) {
	telemetry := telemetrymock.New(t)
	provides, healthPlatform := newTestComponent(t, telemetry)
	component := provides.Observer.(*component)
	advance := useTestClock(&component.detector)
	issueID := dogstatsdclientdrops.UDSIssueIDForHostname(testHostname)

	observe := func(sent, dropped float64) {
		completeUDSWindow(component, clientByteStats{sent: sent, dropped: dropped})
	}

	observe(98, 2)
	advance(component.detector.unhealthyConfirmationDuration)
	observe(100, 0)
	require.Nil(t, healthPlatform.GetIssue(issueID), "a healthy window must cancel a pending open")

	observe(98, 2)
	advance(component.detector.unhealthyConfirmationDuration)
	component.CompleteFinalDogStatsDSerieFlush()
	observe(98, 2)
	require.Nil(t, healthPlatform.GetIssue(issueID), "missing telemetry must cancel a pending open")

	advance(component.detector.unhealthyConfirmationDuration)
	observe(98, 2)
	require.NotNil(t, healthPlatform.GetIssue(issueID))

	observe(100, 0)
	advance(component.detector.recoveryConfirmationDuration)
	observe(98, 2)
	require.NotNil(t, healthPlatform.GetIssue(issueID), "an unhealthy window must cancel a pending resolution")

	observe(100, 0)
	advance(component.detector.recoveryConfirmationDuration)
	component.CompleteFinalDogStatsDSerieFlush()
	observe(100, 0)
	require.NotNil(t, healthPlatform.GetIssue(issueID), "missing telemetry must cancel a pending resolution")

	advance(component.detector.recoveryConfirmationDuration)
	observe(100, 0)
	require.Nil(t, healthPlatform.GetIssue(issueID))
}

func TestComponentReconcilesPersistedIssueState(t *testing.T) {
	t.Run("refreshes current issue and resolves stale hostname", func(t *testing.T) {
		telemetry := telemetrymock.New(t)
		healthPlatform := healthplatformmock.New(t)
		for _, hostname := range []string{testHostname, "previous-node"} {
			issue, err := dogstatsdclientdrops.BuildUDSIssue(dogstatsdclientdrops.UDSDetectionContext{
				Hostname: hostname,
			})
			require.NoError(t, err)
			issue.Id = dogstatsdclientdrops.UDSIssueIDForHostname(hostname)
			require.NoError(t, healthPlatform.ReportIssue(issue))
		}

		component := newTestComponentWithHealthPlatform(t, telemetry, healthPlatform, testHostname).Observer.(*component)
		currentID := dogstatsdclientdrops.UDSIssueIDForHostname(testHostname)
		previousID := dogstatsdclientdrops.UDSIssueIDForHostname("previous-node")
		restoredIssue := healthPlatform.GetIssue(currentID)
		require.NotNil(t, restoredIssue)
		require.Contains(t, restoredIssue.Description, "awaiting current client telemetry")
		require.False(t, restoredIssue.Extra.GetFields()["detection_evidence_available"].GetBoolValue())
		require.Equal(t, []string{previousID}, healthPlatform.ResolvedIDs())
		require.True(t, component.detector.issueNeedsRefresh)

		completeUDSWindow(component, clientByteStats{sent: 98, dropped: 2})

		currentIssue := healthPlatform.GetIssue(currentID)
		require.NotNil(t, currentIssue)
		require.Contains(t, currentIssue.Description, "2.0000%")
		require.Contains(t, currentIssue.Description, "unclassified=2.00")
		require.True(t, currentIssue.Extra.GetFields()["detection_evidence_available"].GetBoolValue())
		require.Equal(t, []string{previousID}, healthPlatform.ResolvedIDs())
		require.False(t, component.detector.issueNeedsRefresh)
	})

	t.Run("rehydrates persisted-only issue before receiving telemetry", func(t *testing.T) {
		telemetry := telemetrymock.New(t)
		issueID := dogstatsdclientdrops.UDSIssueIDForHostname(testHostname)
		baseStore := healthplatformmock.New(t)
		healthPlatform := &persistedOnlyHealthPlatform{
			Mock: baseStore,
			activeByName: map[string][]string{
				dogstatsdclientdrops.UDSIssueName: {issueID},
			},
		}

		component := newTestComponentWithHealthPlatform(t, telemetry, healthPlatform, testHostname).Observer.(*component)
		restoredIssue := baseStore.GetIssue(issueID)
		require.NotNil(t, restoredIssue)
		require.Contains(t, restoredIssue.Description, "awaiting current client telemetry")
		require.False(t, restoredIssue.Extra.GetFields()["detection_evidence_available"].GetBoolValue())
		require.True(t, component.detector.issueActive)
		require.True(t, component.detector.issueNeedsRefresh)

		advance := useTestClock(&component.detector)
		completeUDSWindow(component, clientByteStats{sent: 100})
		require.NotNil(t, baseStore.GetIssue(issueID), "one healthy window must not resolve a restored issue")
		advance(component.detector.recoveryConfirmationDuration)
		completeUDSWindow(component, clientByteStats{sent: 100})

		require.Nil(t, baseStore.GetIssue(issueID))
		require.Equal(t, []string{issueID}, baseStore.ResolvedIDs())
	})

	t.Run("does not resolve absent issue after restart", func(t *testing.T) {
		telemetry := telemetrymock.New(t)
		provides, healthPlatform := newTestComponent(t, telemetry)
		component := provides.Observer.(*component)
		advance := useTestClock(&component.detector)

		completeUDSWindow(component, clientByteStats{sent: 100})
		advance(component.detector.recoveryConfirmationDuration)
		completeUDSWindow(component, clientByteStats{sent: 100})

		require.Empty(t, healthPlatform.ResolvedIDs())
	})
}
