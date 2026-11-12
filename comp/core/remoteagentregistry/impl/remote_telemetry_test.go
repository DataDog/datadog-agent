// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
		key:   exposedSeriesKey{kind: dto.MetricType_COUNTER, name: name, labels: encodeLabels(names, values)},
		meta:  seriesMeta{help: name, labelNames: names, labelValues: values},
		value: seriesValue{counter: value},
	}
}

func histogramObservation(name string, count uint64, sum float64, buckets map[float64]uint64) observedSeries {
	names := []string{emitterMetricTagName}
	values := []string{"adp"}
	return observedSeries{
		key:   exposedSeriesKey{kind: dto.MetricType_HISTOGRAM, name: name, labels: encodeLabels(names, values)},
		meta:  seriesMeta{help: name, labelNames: names, labelValues: values},
		value: seriesValue{count: count, sum: sum, buckets: buckets},
	}
}

// exposedMetrics returns what the accumulator exposes, keyed by metric name followed by the non-emitter label values.
func exposedMetrics(t *testing.T, r *remoteTelemetryAccumulator) map[string]*dto.Metric {
	t.Helper()
	out := make(map[string]*dto.Metric)
	r.emit(nil, func(m prometheus.Metric) {
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

func TestAccumulatorProcessesAreIndependent(t *testing.T) {
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	r.observe("old", []observedSeries{counterObservation("c", 500)}, now)
	r.observe("old", []observedSeries{counterObservation("c", 530)}, now)

	// A restarted remote agent is a new process with its own baseline. Its counters are small, which must not be
	// read as a reset of the old process, and the exposed value must not go down.
	r.observe("new", []observedSeries{counterObservation("c", 2)}, now)
	assert.Equal(t, 30.0, exposedCounter(t, r, "c{}"))
	r.observe("new", []observedSeries{counterObservation("c", 9)}, now)
	assert.Equal(t, 37.0, exposedCounter(t, r, "c{}"))
}

func TestAccumulatorRetainsTotalOfExpiredSeries(t *testing.T) {
	r := newRemoteTelemetryAccumulator()
	now := time.Now()

	r.observe("old", []observedSeries{counterObservation("c", 500)}, now)
	r.observe("old", []observedSeries{counterObservation("c", 530)}, now)

	// The old process is gone for good, and a new one reports the same series.
	later := now.Add(remoteTelemetrySeriesIdleTTL)
	r.observe("new", []observedSeries{counterObservation("c", 1)}, later)
	r.expire(map[string]struct{}{"new": {}}, map[string]struct{}{"new": {}}, later)

	assert.Empty(t, r.processes["old"], "the state of the old process is dropped once nothing refers to it")
	assert.Len(t, r.live, 1)
	assert.Equal(t, 30.0, exposedCounter(t, r, "c{}"))
	r.observe("new", []observedSeries{counterObservation("c", 6)}, later)
	assert.Equal(t, 35.0, exposedCounter(t, r, "c{}"))
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
		assert.Len(t, r.live, 1)
		assert.Empty(t, r.retired)
	})

	t.Run("a series missing from a successful scrape expires and its total is retained", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{"adp": {}}, map[string]struct{}{"adp": {}}, idle)
		assert.Empty(t, r.live)
		assert.Len(t, r.retired, 1)
		assert.Equal(t, 4.0, exposedCounter(t, r, "c{}"))

		// It comes back as a new series of the same process, so all of it is new.
		r.observe("adp", []observedSeries{counterObservation("c", 3)}, idle)
		assert.Equal(t, 7.0, exposedCounter(t, r, "c{}"))
	})

	t.Run("series of a process that left the registry expire", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{}, map[string]struct{}{}, idle)
		assert.Empty(t, r.live)
		assert.Equal(t, 4.0, exposedCounter(t, r, "c{}"))
	})

	t.Run("retained totals expire eventually", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{}, map[string]struct{}{}, idle)
		r.expire(map[string]struct{}{}, map[string]struct{}{}, idle.Add(remoteTelemetryRetiredTTL))
		assert.Empty(t, r.retired)
		assert.Empty(t, r.processes)
		assert.Empty(t, exposedMetrics(t, r))
	})

	t.Run("retained totals that a live series builds on are kept", func(t *testing.T) {
		r := setup()
		r.expire(map[string]struct{}{"adp": {}}, map[string]struct{}{"adp": {}}, idle)
		r.observe("other", []observedSeries{counterObservation("c", 1)}, idle)
		r.observe("other", []observedSeries{counterObservation("c", 2)}, idle)
		r.expire(map[string]struct{}{"other": {}}, map[string]struct{}{"other": {}}, idle.Add(remoteTelemetryRetiredTTL))
		assert.Equal(t, 5.0, exposedCounter(t, r, "c{}"))
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

func TestAccumulatorHistogramLayoutMismatchIsDeterministic(t *testing.T) {
	exposedBounds := func(r *remoteTelemetryAccumulator) []float64 {
		m, ok := exposedMetrics(t, r)["h{}"]
		require.True(t, ok)
		var bounds []float64
		for _, b := range m.GetHistogram().GetBucket() {
			bounds = append(bounds, b.GetUpperBound())
		}
		return bounds
	}

	// Each process reports twice, so its accumulated total is not zero: layout A is {1, +Inf}, layout B {2, +Inf}.
	t.Run("the report seen last wins", func(t *testing.T) {
		r := newRemoteTelemetryAccumulator()
		older, newer := time.Now(), time.Now().Add(time.Minute)

		r.observe("proc-a", []observedSeries{histogramObservation("h", 5, 5, map[float64]uint64{1: 2, math.Inf(1): 5})}, older)
		r.observe("proc-a", []observedSeries{histogramObservation("h", 6, 6, map[float64]uint64{1: 3, math.Inf(1): 6})}, older)
		r.observe("proc-b", []observedSeries{histogramObservation("h", 2, 2, map[float64]uint64{2: 1, math.Inf(1): 2})}, newer)
		r.observe("proc-b", []observedSeries{histogramObservation("h", 3, 3, map[float64]uint64{2: 2, math.Inf(1): 3})}, newer)

		// Whatever the iteration order of the internal maps, the exposed layout is always the newer one.
		for i := 0; i < 10; i++ {
			assert.Equal(t, []float64{2, math.Inf(1)}, exposedBounds(r))
		}
	})

	t.Run("a live series beats a retained total of another layout", func(t *testing.T) {
		r := newRemoteTelemetryAccumulator()
		now := time.Now()
		later := now.Add(2 * time.Minute)

		r.observe("proc-a", []observedSeries{histogramObservation("h", 5, 5, map[float64]uint64{1: 2, math.Inf(1): 5})}, now)
		r.observe("proc-a", []observedSeries{histogramObservation("h", 6, 6, map[float64]uint64{1: 3, math.Inf(1): 6})}, now)
		r.observe("proc-b", []observedSeries{histogramObservation("h", 2, 2, map[float64]uint64{2: 1, math.Inf(1): 2})}, later)
		r.observe("proc-b", []observedSeries{histogramObservation("h", 3, 3, map[float64]uint64{2: 2, math.Inf(1): 3})}, later)

		// The old process leaves and its series is folded later than the new process was last seen.
		r.expire(map[string]struct{}{"proc-b": {}}, map[string]struct{}{"proc-b": {}}, later.Add(remoteTelemetrySeriesIdleTTL-time.Minute))

		// The live series wins even though the retained total is the more recent fold.
		assert.Equal(t, []float64{2, math.Inf(1)}, exposedBounds(r))
	})
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
