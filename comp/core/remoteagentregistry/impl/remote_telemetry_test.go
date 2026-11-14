// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

//
// Unit tests of the accumulator.
//

func counterObservation(name string, value float64, labelValues ...string) observedSeries {
	names := []string{emitterMetricTagName}
	values := []string{"adp"}
	for i, v := range labelValues {
		names = append(names, fmt.Sprintf("l%d", i))
		values = append(values, v)
	}
	return observedSeries{
		key:         exposedSeriesKey{kind: dto.MetricType_COUNTER, name: name, labels: encodeLabels(names, values)},
		help:        name,
		labelNames:  names,
		labelValues: values,
		value:       seriesValue{counter: value},
	}
}

func histogramObservation(name string, count uint64, sum float64, buckets map[float64]uint64) observedSeries {
	names := []string{emitterMetricTagName}
	values := []string{"adp"}
	return observedSeries{
		key:         exposedSeriesKey{kind: dto.MetricType_HISTOGRAM, name: name, labels: encodeLabels(names, values)},
		help:        name,
		labelNames:  names,
		labelValues: values,
		value:       seriesValue{count: count, sum: sum, buckets: buckets},
	}
}

// exposedMetrics returns what the accumulator exposes, keyed by metric name followed by the non-emitter label values.
func exposedMetrics(t *testing.T, r *remoteTelemetryAccumulator) map[string]*dto.Metric {
	t.Helper()
	out := make(map[string]*dto.Metric)
	r.emit(func(m prometheus.Metric) {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		var labels []string
		for _, l := range pb.Label {
			if l.GetName() != emitterMetricTagName {
				labels = append(labels, l.GetValue())
			}
		}
		desc := m.Desc().String()
		name := strings.Split(strings.TrimPrefix(desc, "Desc{fqName: \""), "\"")[0]
		out[name+"{"+strings.Join(labels, ",")+"}"] = &pb
	})
	return out
}

func exposedCounter(t *testing.T, r *remoteTelemetryAccumulator, key string) float64 {
	t.Helper()
	m, ok := exposedMetrics(t, r)[key]
	require.True(t, ok, "series %s is not exposed", key)
	return m.GetCounter().GetValue()
}

func TestAccumulatorFirstScrapeIsBaselineOnly(t *testing.T) {
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	// The remote process has been counting for a long time when the first scrape happens.
	r.observe("adp", []observedSeries{counterObservation("c", 1_000_000)}, now)
	assert.Equal(t, 0.0, exposedCounter(t, r, "c{}"))

	r.observe("adp", []observedSeries{counterObservation("c", 1_000_050)}, now)
	assert.Equal(t, 50.0, exposedCounter(t, r, "c{}"))

	r.observe("adp", []observedSeries{counterObservation("c", 1_000_100)}, now)
	assert.Equal(t, 100.0, exposedCounter(t, r, "c{}"))
}

func TestAccumulatorSeriesAppearingAfterBaselineCountInFull(t *testing.T) {
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	r.observe("adp", []observedSeries{counterObservation("c", 10, "ok")}, now)
	r.observe("adp", []observedSeries{counterObservation("c", 12, "ok"), counterObservation("c", 3, "error")}, now)

	assert.Equal(t, 2.0, exposedCounter(t, r, "c{ok}"))
	assert.Equal(t, 3.0, exposedCounter(t, r, "c{error}"))
}

func TestAccumulatorCounterResetCountsFromZero(t *testing.T) {
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	r.observe("adp", []observedSeries{counterObservation("c", 100)}, now)
	r.observe("adp", []observedSeries{counterObservation("c", 175)}, now)
	// The remote agent dropped and recreated the counter.
	r.observe("adp", []observedSeries{counterObservation("c", 20)}, now)
	assert.Equal(t, 95.0, exposedCounter(t, r, "c{}"))
	r.observe("adp", []observedSeries{counterObservation("c", 35)}, now)
	assert.Equal(t, 110.0, exposedCounter(t, r, "c{}"))
}

func TestAccumulatorExpiryRules(t *testing.T) {
	now := time.Now()
	idle := now.Add(remoteTelemetrySeriesIdleTTL)

	setup := func() *remoteTelemetryAccumulator {
		r := newRemoteTelemetryAccumulator()
		r.observe("adp", []observedSeries{counterObservation("c", 1)}, now)
		r.observe("adp", []observedSeries{counterObservation("c", 5)}, now)
		return r
	}

	t.Run("a failed scrape of a registered process does not expire its series", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{"adp": {}}, map[string]struct{}{}, idle)
		assert.Equal(t, 4.0, exposedCounter(t, r, "c{}"))
	})

	t.Run("a series missing from a successful scrape expires", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{"adp": {}}, map[string]struct{}{"adp": {}}, idle)
		assert.Empty(t, r.series)
		assert.Empty(t, exposedMetrics(t, r))

		// It comes back as a series of a process that is already being watched, so all of it is new.
		r.observe("adp", []observedSeries{counterObservation("c", 3)}, idle)
		assert.Equal(t, 3.0, exposedCounter(t, r, "c{}"))
	})

	t.Run("series of a process that left the registry expire, and so does the process", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{}, map[string]struct{}{}, idle)
		assert.Empty(t, r.series)
		assert.Empty(t, r.processes)
	})

	t.Run("a process that still owns a series is kept", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{}, map[string]struct{}{}, now.Add(remoteTelemetrySeriesIdleTTL-time.Second))
		assert.Len(t, r.series, 1)
		assert.Len(t, r.processes, 1)
	})
}

func TestAccumulatorProcessHandover(t *testing.T) {
	now := time.Now()

	t.Run("the new process takes the series over where the old one left it", func(t *testing.T) {
		r := newRemoteTelemetryAccumulator()
		r.observe("old", []observedSeries{counterObservation("c", 500)}, now)
		r.observe("old", []observedSeries{counterObservation("c", 530)}, now)

		// The old process is gone and a new one reports the same series, starting small.
		r.observe("new", []observedSeries{counterObservation("c", 4)}, now)
		assert.Equal(t, 30.0, exposedCounter(t, r, "c{}"))

		// It climbs past what the old one had reached, which comparing with the old process could not tell apart
		// from continued counting.
		r.observe("new", []observedSeries{counterObservation("c", 900)}, now)
		assert.Equal(t, 30.0+896, exposedCounter(t, r, "c{}"))

		// Nothing refers to the old process anymore.
		r.expire(map[string]struct{}{"new": {}}, map[string]struct{}{"new": {}}, now.Add(remoteTelemetrySeriesIdleTTL))
		assert.NotContains(t, r.processes, "old")
	})

	t.Run("a series a watched process starts reporting later counts in full", func(t *testing.T) {
		r := newRemoteTelemetryAccumulator()
		r.observe("old", []observedSeries{counterObservation("c", 10)}, now)
		r.observe("old", []observedSeries{counterObservation("c", 15)}, now)
		r.observe("new", []observedSeries{counterObservation("other", 1)}, now)

		// "new" is being watched already, and "c" shows up in it now.
		r.observe("new", []observedSeries{counterObservation("other", 2), counterObservation("c", 3)}, now)
		assert.Equal(t, 8.0, exposedCounter(t, r, "c{}"))
	})

	t.Run("a histogram with other buckets starts over", func(t *testing.T) {
		r := newRemoteTelemetryAccumulator()
		layoutA := func(a, total uint64) map[float64]uint64 { return map[float64]uint64{1: a, math.Inf(1): total} }
		layoutB := func(a, b, total uint64) map[float64]uint64 { return map[float64]uint64{2: a, 4: b, math.Inf(1): total} }

		r.observe("old", []observedSeries{histogramObservation("h", 5, 5, layoutA(2, 5))}, now)
		r.observe("old", []observedSeries{histogramObservation("h", 8, 8, layoutA(4, 8))}, now)
		assert.Equal(t, uint64(3), exposedMetrics(t, r)["h{}"].GetHistogram().GetSampleCount())

		// The new process, built with other buckets, is only a baseline for now. What was accounted for the old
		// buckets can't be added to the new ones.
		r.observe("new", []observedSeries{histogramObservation("h", 2, 2, layoutB(1, 1, 2))}, now)
		m := exposedMetrics(t, r)["h{}"].GetHistogram()
		assert.Equal(t, uint64(0), m.GetSampleCount())
		assert.Len(t, m.GetBucket(), 3)

		r.observe("new", []observedSeries{histogramObservation("h", 5, 5, layoutB(2, 3, 5))}, now)
		m = exposedMetrics(t, r)["h{}"].GetHistogram()
		assert.Equal(t, uint64(3), m.GetSampleCount())
	})
}

func TestAccumulatorHistograms(t *testing.T) {
	buckets := func(a, b, c uint64) map[float64]uint64 {
		return map[float64]uint64{0.5: a, 1: b, math.Inf(1): c}
	}
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	r.observe("adp", []observedSeries{histogramObservation("h", 30, 15, buckets(10, 20, 30))}, now)
	m := exposedMetrics(t, r)["h{}"].GetHistogram()
	assert.Equal(t, uint64(0), m.GetSampleCount())

	r.observe("adp", []observedSeries{histogramObservation("h", 36, 18, buckets(12, 25, 36))}, now)
	m = exposedMetrics(t, r)["h{}"].GetHistogram()
	assert.Equal(t, uint64(6), m.GetSampleCount())
	assert.Equal(t, 3.0, m.GetSampleSum())
	got := map[float64]uint64{}
	for _, b := range m.GetBucket() {
		got[b.GetUpperBound()] = b.GetCumulativeCount()
	}
	assert.Equal(t, buckets(2, 5, 6), got)

	// A reset is counted from zero instead of wrapping around.
	r.observe("adp", []observedSeries{histogramObservation("h", 4, 2, buckets(1, 2, 4))}, now)
	m = exposedMetrics(t, r)["h{}"].GetHistogram()
	assert.Equal(t, uint64(10), m.GetSampleCount())
	assert.Equal(t, 5.0, m.GetSampleSum())

	// A different bucket layout starts a new histogram.
	other := map[float64]uint64{2: 1, math.Inf(1): 3}
	r.observe("adp", []observedSeries{histogramObservation("h", 3, 9, other)}, now)
	m = exposedMetrics(t, r)["h{}"].GetHistogram()
	assert.Equal(t, uint64(3), m.GetSampleCount())
	assert.Len(t, m.GetBucket(), 2)
}

//
// Tests through the registry.
//

// registerTelemetryAgent registers a remote agent process, and returns once the registry has taken the baseline of its
// telemetry. promText is what the process has reported by then.
func registerTelemetryAgent(t *testing.T, ipcComp ipc.Component, registryComp remoteagent.Component, processInstanceID string, promText string) *testRemoteAgentServer {
	t.Helper()
	server := buildRemoteAgent(t, ipcComp, "agent_data_plane", "Agent Data Plane", "123", withTelemetryProvider(promText))
	server.RegistrationData.ProcessInstanceID = processInstanceID
	sessionID, _, err := registryComp.RegisterRemoteAgent(&server.RegistrationData)
	require.NoError(t, err)
	server.setRegisteredSessionID(sessionID)
	waitForTelemetryBaseline(t, registryComp, sessionID)
	return server
}

func counterText(name string, value float64) string {
	return fmt.Sprintf("# TYPE %s counter\n%s %v\n", name, name, value)
}

func gatheredCounter(t *testing.T, telemetryComp telemetry.Component, name string) float64 {
	t.Helper()
	families, err := telemetryComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			require.Len(t, family.GetMetric(), 1)
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	require.Failf(t, "metric not found", "metric %s was not gathered", name)
	return 0
}

// TestTelemetryCoreAgentRestartDoesNotReportRemoteLifetimeCounters is the scenario this logic exists for: the
// remote agent has been running for a long time when a (new) Core Agent starts.
func TestTelemetryCoreAgentRestartDoesNotReportRemoteLifetimeCounters(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	// The remote agent registers with the Core Agent that just started, holding the total it has accumulated.
	server := registerTelemetryAgent(t, ipcComp, provides.Comp, "adp-process", counterText("adp_counter", 1_000_000))

	// What the Core Agent reports is what was counted since it first saw the process.
	assert.Equal(t, 0.0, gatheredCounter(t, telemetryComp, "adp_counter"))

	server.promText = counterText("adp_counter", 1_000_050)
	assert.Equal(t, 50.0, gatheredCounter(t, telemetryComp, "adp_counter"))

	server.promText = counterText("adp_counter", 1_000_100)
	assert.Equal(t, 100.0, gatheredCounter(t, telemetryComp, "adp_counter"))
}

func TestTelemetryReRegistrationOfSameProcessKeepsCounting(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	server := registerTelemetryAgent(t, ipcComp, provides.Comp, "adp-process", counterText("adp_counter", 500))
	server.promText = counterText("adp_counter", 530)
	assert.Equal(t, 30.0, gatheredCounter(t, telemetryComp, "adp_counter"))

	// The remote agent registers again with a new session, as it does when a refresh fails. It is the same process,
	// so what it counted while it was not registered is still counted.
	server.promText = counterText("adp_counter", 541)
	sessionID, _, err := provides.Comp.RegisterRemoteAgent(&server.RegistrationData)
	require.NoError(t, err)
	server.setRegisteredSessionID(sessionID)
	waitForTelemetryBaseline(t, provides.Comp, sessionID)

	assert.Equal(t, 41.0, gatheredCounter(t, telemetryComp, "adp_counter"))
}

func TestTelemetryRemoteAgentRestartKeepsCountersMonotonic(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	old := registerTelemetryAgent(t, ipcComp, provides.Comp, "first-process", counterText("adp_counter", 500))
	old.promText = counterText("adp_counter", 530)
	assert.Equal(t, 30.0, gatheredCounter(t, telemetryComp, "adp_counter"))

	// The remote agent restarts: a new process with small counters. The old process is gone but still registered
	// until it times out.
	old.Stop()
	restarted := registerTelemetryAgent(t, ipcComp, provides.Comp, "second-process", counterText("adp_counter", 4))
	assert.Equal(t, 30.0, gatheredCounter(t, telemetryComp, "adp_counter"))

	// The new process counts past what the old one had reached, which a plain comparison with the last total could
	// not tell from continued counting.
	restarted.promText = counterText("adp_counter", 900)
	assert.Equal(t, 30.0+896, gatheredCounter(t, telemetryComp, "adp_counter"))
}

func TestTelemetrySeriesAppearingLaterCountInFull(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	server := registerTelemetryAgent(t, ipcComp, provides.Comp, "adp-process", counterText("adp_counter", 10))
	server.promText = counterText("adp_counter", 10) + counterText("adp_errors", 3)

	assert.Equal(t, 0.0, gatheredCounter(t, telemetryComp, "adp_counter"))
	assert.Equal(t, 3.0, gatheredCounter(t, telemetryComp, "adp_errors"))
}

func TestTelemetryHistogramIsRebased(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	histogramText := func(a, b, count uint64, sum float64) string {
		return fmt.Sprintf(`# TYPE adp_latency histogram
adp_latency_bucket{le="1"} %d
adp_latency_bucket{le="+Inf"} %d
adp_latency_sum %v
adp_latency_count %d
`, a, b, sum, count)
	}
	gatherHistogram := func() *dto.Histogram {
		families, err := telemetryComp.Gather(telemetry.NoFilter)
		require.NoError(t, err)
		for _, family := range families {
			if family.GetName() == "adp_latency" {
				return family.GetMetric()[0].GetHistogram()
			}
		}
		require.Fail(t, "histogram not found")
		return nil
	}

	server := registerTelemetryAgent(t, ipcComp, provides.Comp, "adp-process", histogramText(90, 100, 100, 50))
	assert.Equal(t, uint64(0), gatherHistogram().GetSampleCount())

	server.promText = histogramText(95, 110, 110, 56)
	h := gatherHistogram()
	assert.Equal(t, uint64(10), h.GetSampleCount())
	assert.Equal(t, 6.0, h.GetSampleSum())
	assert.Equal(t, uint64(5), h.GetBucket()[0].GetCumulativeCount())

	// A reset must not wrap around.
	server.promText = histogramText(1, 2, 2, 1)
	h = gatherHistogram()
	assert.Equal(t, uint64(12), h.GetSampleCount())
	assert.Equal(t, uint64(6), h.GetBucket()[0].GetCumulativeCount())
}

func TestTelemetryGaugesAreNotRebased(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	server := registerTelemetryAgent(t, ipcComp, provides.Comp, "adp-process", "# TYPE adp_gauge gauge\nadp_gauge 7\n")
	families, err := telemetryComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	var found bool
	for _, family := range families {
		if family.GetName() == "adp_gauge" {
			found = true
			assert.Equal(t, 7.0, family.GetMetric()[0].GetGauge().GetValue())
		}
	}
	assert.True(t, found)

	server.promText = "# TYPE adp_gauge gauge\nadp_gauge 2\n"
	families, err = telemetryComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == "adp_gauge" {
			assert.Equal(t, 2.0, family.GetMetric()[0].GetGauge().GetValue())
		}
	}
}

// TestTelemetryBaselineScrapeToleratesRemoteAgentNotKnowingItsSessionYet covers the window right after registration,
// in which a remote agent may answer with a session ID that isn't the registered one yet. That is expected, so it must
// not get the agent deregistered or show up as an error, and the baseline is taken once the agent gets it right.
func TestTelemetryBaselineScrapeToleratesRemoteAgentNotKnowingItsSessionYet(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())
	registry := provides.Comp.(*remoteAgentRegistry)

	server := buildRemoteAgent(t, ipcComp, "agent_data_plane", "Agent Data Plane", "123",
		withTelemetryProvider(counterText("adp_counter", 1_000)),
		withFakeSessionID("not-registered-yet"),
	)
	sessionID, _, err := registry.RegisterRemoteAgent(&server.RegistrationData)
	require.NoError(t, err)
	registry.agentMapMu.Lock()
	client := registry.agentMap[sessionID]
	identity := remoteAgentIdentity(client.RegisteredAgent)
	registry.agentMapMu.Unlock()

	// Wait until the registry has attempted the baseline scrape at least twice against the wrong session ID.
	require.Eventually(t, func() bool { return server.telemetryCalls.Load() >= 2 }, 10*time.Second, 5*time.Millisecond)

	// The expected failures left no trace: the agent is not marked unhealthy, no error was counted, and the wrong
	// session ID did not become the baseline.
	client.unhealthyMu.Lock()
	unhealthy := client.unhealthyReason
	client.unhealthyMu.Unlock()
	require.NoError(t, unhealthy, "the agent must not be marked unhealthy by a baseline scrape")
	require.False(t, registry.telemetryAccumulator.isBaselined(identity), "a wrong session ID must not become the baseline")

	// The remote agent learns its session ID, and the baseline succeeds.
	server.setRegisteredSessionID(sessionID)
	server.clearSessionOverride()
	require.Eventually(t, func() bool { return registry.telemetryAccumulator.isBaselined(identity) }, 10*time.Second, 5*time.Millisecond)

	// What was counted before the baseline is not reported, and no attempt ever counted an error.
	families, err := telemetryComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	for _, family := range families {
		assert.NotContains(t, family.GetName(), "action_error", "baseline attempts must not be counted as errors")
	}

	server.promText = counterText("adp_counter", 1_040)
	assert.Equal(t, 40.0, gatheredCounter(t, telemetryComp, "adp_counter"))
}

// Two registrations under one agent name report the same series, so only the newest one is scraped: the older one is
// the one that went away. If both were scraped, the series would change owner on every scrape.
func TestTelemetryOnlyTheNewestRegistrationOfANameIsScraped(t *testing.T) {
	provides, lc, _, telemetryComp, ipcComp := buildComponent(t)
	require.NoError(t, lc.Start(context.Background()))
	defer lc.Stop(context.Background())

	old := registerTelemetryAgent(t, ipcComp, provides.Comp, "first-process", counterText("adp_counter", 100))
	// The old process is still registered, and still answers with its own numbers.
	restarted := registerTelemetryAgent(t, ipcComp, provides.Comp, "second-process", counterText("adp_counter", 7))

	old.promText = counterText("adp_counter", 105)
	restarted.promText = counterText("adp_counter", 9)

	assert.Equal(t, 2.0, gatheredCounter(t, telemetryComp, "adp_counter"))
}

// A fake avoids sockets and lets the test supersede a registration at an exact point in its baseline RPC.
type baselineTelemetryClient struct {
	session string
	get     func(context.Context) (string, error)
}

func (c *baselineTelemetryClient) GetTelemetry(ctx context.Context, _ *pb.GetTelemetryRequest, opts ...grpc.CallOption) (*pb.GetTelemetryResponse, error) {
	text, err := c.get(ctx)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		if header, ok := opt.(grpc.HeaderCallOption); ok {
			*header.HeaderAddr = metadata.Pairs("session_id", c.session)
		}
	}
	return &pb.GetTelemetryResponse{Payload: &pb.GetTelemetryResponse_PromText{PromText: text}}, nil
}

func TestTelemetryBaselineSkipsSupersededRegistration(t *testing.T) {
	for _, inFlight := range []bool{false, true} {
		name := "retry after replacement baselines"
		if inFlight {
			name = "replacement registers during baseline RPC"
		}
		t.Run(name, func(t *testing.T) {
			provides, _, _, _, _ := buildComponent(t)
			registry := provides.Comp.(*remoteAgentRegistry)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			started, release := make(chan struct{}), make(chan struct{})
			calls := 0
			old := &remoteAgentClient{
				RegisteredAgent: remoteagent.RegisteredAgent{
					Flavor: adpAgentFlavor, SanitizedDisplayName: "agent-data-plane", SessionID: "old", ProcessInstanceID: "process-a",
				},
				registrationOrder: 1,
				services:          []string{TelemetryServiceName},
				TelemetryProviderClient: &baselineTelemetryClient{session: "old", get: func(context.Context) (string, error) {
					calls++
					if inFlight {
						close(started)
						select {
						case <-release:
						case <-ctx.Done():
							return "", ctx.Err()
						}
					} else if calls == 1 {
						return "", errors.New("telemetry not ready")
					}
					return counterText("adp_counter", 500), nil
				}},
			}
			registry.agentMap[old.SessionID] = old

			var done chan bool
			if inFlight {
				done = make(chan bool, 1)
				go func() { done <- registry.primeTelemetryBaselineOnce(old.SessionID) }()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("baseline RPC did not start")
				}
			} else {
				require.False(t, registry.primeTelemetryBaselineOnce(old.SessionID))
			}

			raw := 100.0
			newer := &remoteAgentClient{
				RegisteredAgent: remoteagent.RegisteredAgent{
					Flavor: adpAgentFlavor, SanitizedDisplayName: "agent-data-plane", SessionID: "new", ProcessInstanceID: "process-b",
				},
				registrationOrder: 2,
				services:          []string{TelemetryServiceName},
				TelemetryProviderClient: &baselineTelemetryClient{session: "new", get: func(context.Context) (string, error) {
					return counterText("adp_counter", raw), nil
				}},
			}
			registry.agentMapMu.Lock()
			registry.agentMap[newer.SessionID] = newer
			registry.agentMapMu.Unlock()

			if inFlight {
				close(release)
				select {
				case stopped := <-done:
					require.True(t, stopped)
				case <-ctx.Done():
					t.Fatal("superseded baseline did not finish")
				}
				require.False(t, registry.telemetryAccumulator.isBaselined(remoteAgentIdentity(old.RegisteredAgent)))
			}

			require.True(t, registry.primeTelemetryBaselineOnce(newer.SessionID))
			require.True(t, registry.primeTelemetryBaselineOnce(old.SessionID), "a superseded registration must stop retrying")
			require.Equal(t, 1, calls, "a superseded registration must not be queried again")

			raw = 110
			registry.collectTelemetry(registry.newestRegistrationFilter(), func(prometheus.Metric) {})
			require.Equal(t, 10.0, exposedCounter(t, registry.telemetryAccumulator, "adp_counter{}"), "only the increase since the replacement's baseline is new")
		})
	}
}
