// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserverTelemetry_NoopsDoNotPanic(_ *testing.T) {
	tel := newObserverTelemetry(noopsimpl.NewComponent())
	tel.recordObservationAccepted("logs", "containers")
	tel.recordObservationDropped("logs", "containers")
	tel.setLogPatternCount(1)
	tel.recordLogAccepted("internal", 256)
	tel.recordFilteredLog()
	tel.recordMetricAccepted("dogstatsd")
	tel.recordFilteredMetric("dogstatsd")
	tel.incrementLogsInFlight("internal")
	tel.decrementLogsInFlight("internal")
	tel.initLogsInFlight()
	tel.setSeriesCount(42)
	tel.recordStorageSeriesEvicted("capacity", 3)
	tel.recordStorageCapacityHit()
	tel.recordAnomalyDedupEvicted(anomalyDedupEvictionReasonCapacity, 2)
	tel.recordAnomalyDedupEvicted(anomalyDedupEvictionReasonRetention, 1)
	tel.recordAdvanceSkipped("input")
	tel.recordInputRateLimiterDropped("internal", "high")
	tel.recordFreshSeries(3)
	tel.recordDetectorEmission("bocpd", "unset", "medium", 1)
	tel.scorerSeverity.Set(2, "anomaly_scorer")
}

func TestObserverTelemetry_EmitsNewMetrics(t *testing.T) {
	telComp := telemetryimpl.NewMock(t)

	tel := newObserverTelemetry(telComp)
	tel.recordLogAccepted("kubelet", 128)
	tel.recordFilteredLog()
	tel.recordMetricAccepted("dogstatsd")
	tel.recordObservationDropped("logs", "containers")
	tel.recordInputRateLimiterDropped("internal", "high")
	tel.recordDetectorEmission("bocpd", "unset", "medium", 1)
	tel.recordDetectorEmission("bocpd", "unset", "medium", 1)
	tel.recordGatedDetections("stable_pipeline:scanmw", "stable", "relative_change", 2)
	tel.recordAnomalyDedupEvicted(anomalyDedupEvictionReasonCapacity, 3)
	tel.recordAnomalyDedupEvicted(anomalyDedupEvictionReasonSeries, 4)
	tel.recordAnomalyDedupEvicted(anomalyDedupEvictionReasonRetention, 5)
	tel.setLogPatternCount(3)
	tel.recordFreshSeries(2)
	tel.scorerSeverity.Set(2, "anomaly_scorer")

	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsAccepted, map[string]string{"kind": "logs", "source": "kubelet"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsAccepted, map[string]string{"kind": "metrics", "source": "dogstatsd"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryObservationsDropped, map[string]string{"kind": "logs", "source": "containers"}).GetCounter().GetValue())
	assert.Equal(t, 128.0, observerMetric(t, telComp, telemetryLogsAcceptedBytes, map[string]string{"source": "kubelet"}).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryFilteredLogs, nil).GetCounter().GetValue())
	assert.Equal(t, 1.0, observerMetric(t, telComp, telemetryLogsInputRateLimiterDropped, map[string]string{"source": "internal", "priority": "high"}).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryKindEmissions, map[string]string{"detector": "bocpd", "kind": "unset", "severity": "medium"}).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryGatedDetections,
		map[string]string{"detector": "stable_pipeline:scanmw", "kind": "stable", "gate": "relative_change"}).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryDetectorEmissions, map[string]string{"detector": "bocpd", "severity": "medium"}).GetCounter().GetValue())
	assert.Equal(t, 3.0, observerMetric(t, telComp, telemetryAnomalyDedupEvicted, map[string]string{"reason": anomalyDedupEvictionReasonCapacity}).GetCounter().GetValue())
	assert.Equal(t, 4.0, observerMetric(t, telComp, telemetryAnomalyDedupEvicted, map[string]string{"reason": anomalyDedupEvictionReasonSeries}).GetCounter().GetValue())
	assert.Equal(t, 5.0, observerMetric(t, telComp, telemetryAnomalyDedupEvicted, map[string]string{"reason": anomalyDedupEvictionReasonRetention}).GetCounter().GetValue())
	assert.Equal(t, 3.0, observerMetric(t, telComp, telemetryLogPatternExtractorPatternCount, nil).GetGauge().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetrySeriesFresh, nil).GetCounter().GetValue())
	assert.Equal(t, 2.0, observerMetric(t, telComp, telemetryScorerSeverity, map[string]string{"scorer": "anomaly_scorer"}).GetGauge().GetValue())
}

func observerMetric(t *testing.T, telemetryComp telemetry.Component, metricName string, wantLabels map[string]string) *dto.Metric {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(telemetry.NoFilter)
	require.NoError(t, err)

	fullMetricName := "observer__" + metricName
	for _, family := range metricFamilies {
		if family.GetName() != fullMetricName {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if len(labels) != len(wantLabels) {
				continue
			}
			matches := true
			for name, value := range wantLabels {
				if labels[name] != value {
					matches = false
					break
				}
			}
			if matches {
				return metric
			}
		}
	}

	t.Fatalf("metric %q with labels %v not found", fullMetricName, wantLabels)
	return nil
}

func TestEngineEmissionTelemetryBatching(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	e, _ := makeEngine([]observer.Anomaly{
		makeTestAnomaly("metric_a", 100),
		makeTestAnomaly("metric_b", 100),
	})

	e.Advance(310)
	e.publishMetricKindTelemetry(tel)
	labels := map[string]string{"detector": "scanmw", "kind": "unset", "severity": "xlow"}
	require.Equal(t, 2.0, observerMetric(t, comp, telemetryKindEmissions, labels).GetCounter().GetValue())
	require.Equal(t, 2.0, observerMetric(t, comp, telemetryDetectorEmissions,
		map[string]string{"detector": "scanmw", "severity": "xlow"}).GetCounter().GetValue())
	require.Empty(t, e.emissionCounts, "emissions must be cleared after a drain")

	before, err := comp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	e.publishMetricKindTelemetry(tel)
	after, err := comp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	require.Empty(t, e.emissionCounts)
	require.Len(t, before, len(after), "an idle drain must not publish new emission samples")
}

func TestEngineEmissionTelemetryLifecycle(t *testing.T) {
	e, _ := makeEngine([]observer.Anomaly{makeTestAnomaly("metric_a", 100)})

	e.Advance(310)
	require.Len(t, e.emissionCounts, 1)
	e.Reset()
	require.Empty(t, e.emissionCounts, "reset must clear pending emissions")

	e, _ = makeEngine([]observer.Anomaly{makeTestAnomaly("metric_a", 100)})
	e.Advance(310)
	require.Len(t, e.emissionCounts, 1)
	e.SetDetectors(nil)
	require.Empty(t, e.emissionCounts, "component replacement must clear pending emissions")
}

func TestClassifyLogSource(t *testing.T) {
	require.Equal(t, "internal", classifyLogSource("agent_logs", nil))
	require.Equal(t, "kubelet", classifyLogSource("logs", []string{"source:kubelet", "service:kubelet"}))
	require.Equal(t, "containers", classifyLogSource("logs", []string{"source:docker"}))
}

func TestMetricKindTelemetryPublication(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	s := newDetectorTestStorage()
	ref := addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
	e, p, _ := newPipelineEngine(t, s, observer.STABLE, observer.UNKNOWN)
	e.publishMetricKindTelemetry(tel)
	for kind := observer.UNSET; kind <= observer.LINEAR; kind++ {
		want := 0.0
		if kind == observer.UNSET {
			want = 1
		}
		require.Equal(t, want, observerMetric(t, comp, telemetryMetricKindSeriesCount,
			map[string]string{"kind": kind.String()}).GetGauge().GetValue())
	}
	p.advanceKinds(s, 3)
	p.advanceKinds(s, 3)
	p.registrations[0].timing.record(10 * time.Nanosecond)
	p.registrations[0].timing.record(30 * time.Nanosecond)
	e.publishMetricKindTelemetry(tel)
	transitions := map[string]string{"from": "unset", "to": "stable"}
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryMetricKindTransitions, transitions).GetCounter().GetValue())
	labels := map[string]string{"detector": "pipeline", "kind": "stable"}
	require.Equal(t, 20.0, observerMetric(t, comp, telemetryKindProcessingTimeNs, labels).GetGauge().GetValue())
	require.Equal(t, 2.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())
	e.publishMetricKindTelemetry(tel)
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryMetricKindTransitions, transitions).GetCounter().GetValue())
	require.Equal(t, 2.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())

	addPipelinePoints(s, 1, 4, 4, metrics.GaugeType)
	p.advanceKinds(s, 4)
	e.publishMetricKindTelemetry(tel)
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryMetricKindTransitions,
		map[string]string{"from": "stable", "to": "unknown"}).GetCounter().GetValue())
	require.Equal(t, 0.0, observerMetric(t, comp, telemetryMetricKindSeriesCount,
		map[string]string{"kind": "stable"}).GetGauge().GetValue())
	s.RemoveSeriesByRefs([]observer.SeriesRef{ref})
	e.fanOutSeriesRemoval([]observer.SeriesRef{ref})
	e.publishMetricKindTelemetry(tel)
	require.Equal(t, 0.0, observerMetric(t, comp, telemetryMetricKindSeriesCount,
		map[string]string{"kind": "unknown"}).GetGauge().GetValue())
}

func TestMetricPipelineTelemetryResetAndReplacement(t *testing.T) {
	s := newDetectorTestStorage()
	addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
	e, p, _ := newPipelineEngine(t, s, observer.STABLE)
	p.advanceKinds(s, 3)
	require.Equal(t, uint64(1), p.kindTransitions[observer.UNSET][observer.STABLE])
	p.Detect(s, 3, observer.DetectionFilter{})
	require.Equal(t, uint64(1), p.registrations[0].timing.calls)
	e.Reset()
	require.Equal(t, metricKindTransitionCounts{}, p.drainKindTransitionCounts())
	require.Zero(t, p.registrations[0].timing.calls)
	require.Equal(t, [metricKindCount]uint64{1, 0, 0, 0}, s.SeriesCountByKind())
	p.advanceKinds(s, 3)
	e.resetAnalysisState()
	require.Equal(t, metricKindTransitionCounts{}, p.drainKindTransitionCounts())
	p.recordKindTransition(observer.UNKNOWN, observer.STABLE)
	replacement := newScanMWPipeline(t, DefaultScanMWConfig())
	e.SetDetectors([]detectorRegistration{{detector: replacement}})
	comp := telemetryimpl.NewMock(t)
	e.publishMetricKindTelemetry(newObserverTelemetry(comp))
	require.Equal(t, uint64(1), p.kindTransitions[observer.UNKNOWN][observer.STABLE],
		"flush must only drain current registrations")
	replacement.recordKindTransition(observer.UNSET, observer.STABLE)
	replacement.registrations[0].timing.record(time.Nanosecond)
	e.ResetForReplay(e.detectors, nil, nil, nil, s.cfg, BaselineConfig{})
	require.Equal(t, metricKindTransitionCounts{}, replacement.drainKindTransitionCounts())
	require.Zero(t, replacement.registrations[0].timing.calls)
	require.Equal(t, [metricKindCount]uint64{}, e.storage.SeriesCountByKind())
}

func TestMetricPipelineEmissionTelemetry(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	s := newDetectorTestStorage()
	ref := addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
	e, p, d := newPipelineEngine(t, s, observer.STABLE)
	d.ref = ref
	result := e.Advance(3)
	require.Len(t, result.anomalies, 1)
	require.Equal(t, observer.STABLE, result.anomalies[0].MetricKind)
	require.Equal(t, d.Name(), result.anomalies[0].DetectorName)
	e.Advance(4)
	p.series[ref].reactivated = true
	p.series[ref].outputAfter = 4
	require.Empty(t, e.Advance(5).anomalies)
	meanNs := float64(p.registrations[0].timing.duration.Nanoseconds()) / 3
	e.publishMetricKindTelemetry(tel)
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryKindEmissions,
		map[string]string{"detector": d.Name(), "kind": "stable", "severity": "medium"}).GetCounter().GetValue())
	require.Empty(t, e.drainEmissionCounts(), "emissions must be cleared after a drain")
	require.Equal(t, 3.0, observerMetric(t, comp, telemetryKindProcessingCalls,
		map[string]string{"detector": d.Name(), "kind": "stable"}).GetCounter().GetValue())
	require.Equal(t, meanNs, observerMetric(t, comp, telemetryKindProcessingTimeNs,
		map[string]string{"detector": d.Name(), "kind": "stable"}).GetGauge().GetValue())
}

func TestMetricWorkflowTimingAndBaselineTelemetry(t *testing.T) {
	for _, pipeline := range []bool{false, true} {
		t.Run(fmt.Sprintf("pipeline=%t", pipeline), func(t *testing.T) {
			comp := telemetryimpl.NewMock(t)
			tel := newObserverTelemetry(comp)
			s := newDetectorTestStorage()
			ref := addPipelinePoints(s, 1, 1, 3, metrics.GaugeType)
			e, p, d := newPipelineEngine(t, s, observer.STABLE)
			d.ref = ref
			kind := "stable"
			if !pipeline {
				kind = "unset"
				e.SetDetectors([]detectorRegistration{{detector: d}})
			}
			e.baseline = newBaselineController(BaselineConfig{
				Enabled: true, DurationSec: 10, MuteNoisyMetrics: true,
			}, detectorNames(e.detectors))
			require.Empty(t, e.Advance(3).anomalies)
			e.Advance(4)
			require.Empty(t, e.drainEmissionCounts(), "baseline emissions must not be counted")
			timing := p.registrations[0].timing
			if !pipeline {
				timing = *e.detectors[0].timing
			}
			e.publishMetricKindTelemetry(tel)
			labels := map[string]string{"detector": d.Name(), "kind": kind}
			require.Equal(t, 2.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())
			require.Equal(t, float64(timing.duration.Nanoseconds())/2,
				observerMetric(t, comp, telemetryKindProcessingTimeNs, labels).GetGauge().GetValue())
			e.Reset()
			e.publishMetricKindTelemetry(tel)
			require.Equal(t, 2.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())
		})
	}
}

func TestMetricPipelineInnerDetectorTimings(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	stable, _ := newPipelineStub(observer.STABLE, "scanmw")
	linear, _ := newPipelineStub(observer.LINEAR, "linear_detector")
	p, err := newMetricPipeline([]pipelineRegistration{stable, linear}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	e := mustNewEngine(engineConfig{
		storage:   newDetectorTestStorage(),
		detectors: []detectorRegistration{{detector: p}},
	})
	e.Advance(1)
	timings := []detectorTiming{p.registrations[0].timing, p.registrations[1].timing}
	e.publishMetricKindTelemetry(tel)
	for i, registration := range p.registrations {
		labels := map[string]string{"detector": registration.detector.Name(), "kind": registration.kind.String()}
		require.Equal(t, 1.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())
		require.Equal(t, float64(timings[i].duration.Nanoseconds()),
			observerMetric(t, comp, telemetryKindProcessingTimeNs, labels).GetGauge().GetValue())
	}
	require.Zero(t, e.detectors[0].timing.calls, "the wrapper must not be timed again")
	e.publishMetricKindTelemetry(tel)
	require.Zero(t, observerMetric(t, comp, telemetryKindProcessingTimeNs,
		map[string]string{"detector": "scanmw", "kind": "stable"}).GetGauge().GetValue(), "idle intervals publish zero")
}

func TestDetectorTimingZeroDuration(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	var timing detectorTiming
	timing.record(0) // Sub-clock-resolution calls still count.
	timing.publish(tel, "scanmw", "stable")
	labels := map[string]string{"detector": "scanmw", "kind": "stable"}
	require.Zero(t, observerMetric(t, comp, telemetryKindProcessingTimeNs, labels).GetGauge().GetValue())
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryKindProcessingCalls, labels).GetCounter().GetValue())
	require.Equal(t, detectorTiming{}, timing)
}

func TestMetricPipelineTransitionCounts(t *testing.T) {
	p := newScanMWPipeline(t, DefaultScanMWConfig())
	p.recordKindTransition(observer.UNSET, observer.STABLE)
	p.recordKindTransition(observer.UNSET, observer.STABLE)
	p.recordKindTransition(observer.STABLE, observer.UNKNOWN)
	p.recordKindTransition(observer.STABLE, observer.STABLE)
	p.recordKindTransition(observer.MetricKind(255), observer.STABLE)
	counts := p.drainKindTransitionCounts()
	require.Equal(t, uint64(2), counts[observer.UNSET][observer.STABLE])
	require.Equal(t, uint64(1), counts[observer.STABLE][observer.UNKNOWN])
	require.Zero(t, counts[observer.STABLE][observer.STABLE])
	require.Equal(t, metricKindTransitionCounts{}, p.drainKindTransitionCounts())
}

func TestMetricWorkflowMutedEmissionTelemetry(t *testing.T) {
	for _, pipeline := range []bool{false, true} {
		t.Run(fmt.Sprintf("pipeline=%t", pipeline), func(t *testing.T) {
			s := newDetectorTestStorage()
			addPipelineValues(s, 1, 1, 40, 10)
			var detector observer.Detector = NewScanMWDetector(testScanMWPipelineConfig())
			if pipeline {
				detector = newScanMWPipeline(t, testScanMWPipelineConfig())
			}
			e := mustNewEngine(engineConfig{
				storage:                    s,
				detectors:                  []detectorRegistration{{detector: detector}},
				baseline:                   BaselineConfig{Enabled: true, DurationSec: 100, MuteNoisyMetrics: true},
				trackDetectorOutputHistory: true,
			})
			e.Advance(40)
			addPipelineValues(s, 1, 41, 60, 100)
			e.Advance(60)
			require.NotEmpty(t, e.DetectorOutputAnomalies(), "the baseline must see real detector outputs")
			e.Advance(200)
			require.NotEmpty(t, e.baseline.mutedHashes)
			require.Zero(t, s.TotalSeriesCount())
			require.Empty(t, e.drainEmissionCounts(), "baseline qualification and noisy-metric muting must suppress telemetry")
		})
	}
}
func TestMetricPipelineGatedTelemetry(t *testing.T) {
	comp := telemetryimpl.NewMock(t)
	tel := newObserverTelemetry(comp)
	inner := &stableStubDetector{name: "scanmw", ready: true, anomalies: []observer.Anomaly{
		stableEvidenceAnomaly(100, 103), // below threshold
		stableEvidenceAnomaly(100, 110), // accepted
	}}
	stable, err := newStablePipeline(inner, DefaultStablePipelineConfig())
	require.NoError(t, err)
	registration := pipelineRegistration{
		kind:         observer.STABLE,
		metricType:   metrics.GaugeType,
		detector:     stable,
		aggregations: []observer.Aggregate{observer.AggregateAverage},
	}
	p, err := newMetricPipeline([]pipelineRegistration{registration}, NewMetricKindMatcher(DefaultMetricKindMatcherConfig()))
	require.NoError(t, err)
	e := mustNewEngine(engineConfig{
		storage:                    newDetectorTestStorage(),
		detectors:                  []detectorRegistration{{detector: p}},
		trackDetectorOutputHistory: true,
	})

	result := p.Detect(nil, 1, observer.DetectionFilter{})
	require.Len(t, result.Anomalies, 1)

	e.publishMetricKindTelemetry(tel)
	labels := map[string]string{"detector": "stable_pipeline:scanmw", "kind": "stable", "gate": "relative_change"}
	require.Equal(t, 1.0, observerMetric(t, comp, telemetryGatedDetections, labels).GetCounter().GetValue())

	before, err := comp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	e.publishMetricKindTelemetry(tel)
	after, err := comp.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	require.Empty(t, stable.drainGatedRejections(), "drain must clear pending rejections")
	require.Len(t, before, len(after), "an idle drain must not publish new gated samples")
}
