// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

// fixedDetector is a mock detector that returns a fixed set of anomalies once.
type fixedDetector struct {
	anomalies []observer.Anomaly
	fired     bool
}

func (d *fixedDetector) Name() string { return "fixed" }

func (d *fixedDetector) Detect(_ observer.StorageReader, _ int64) observer.DetectionResult {
	if d.fired {
		return observer.DetectionResult{}
	}
	d.fired = true
	return observer.DetectionResult{Anomalies: d.anomalies}
}

func makeTestAnomaly(name string, ts int64) observer.Anomaly {
	return observer.Anomaly{
		Source:       observer.SeriesDescriptor{Namespace: "ns", Name: name, Aggregate: observer.AggregateAverage},
		DetectorName: "scanmw",
		Timestamp:    ts,
		Description:  name + " changed",
	}
}

func makeEngine(anomalies []observer.Anomaly) (*engine, *TimeClusterCorrelator) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 400; sec++ {
		storage.Add("ns", "metric_a", 100.0, sec, nil)
		storage.Add("ns", "metric_b", 100.0, sec, nil)
	}

	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())
	detector := &fixedDetector{anomalies: anomalies}

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observer.Detector{detector},
		correlators: []observer.Correlator{correlator},
	})
	return e, correlator
}

// TestStepAdvance_SingleTimestampGroupFarBehindUpTo tests that clusters are
// accumulated even when all anomalies share one timestamp >WindowSeconds behind upTo.
// This is the common case for scan detectors in live: multiple metrics change at
// the same second, detected 150-300s later.
func TestStepAdvance_SingleTimestampGroupFarBehindUpTo(t *testing.T) {
	e, _ := makeEngine([]observer.Anomaly{
		makeTestAnomaly("metric_a", 100),
		makeTestAnomaly("metric_b", 100),
	})

	// upTo=310, anomalies at 100 → 210s gap, well past 120s window.
	e.Advance(310)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated: %d correlations", len(accumulated))
	for _, ac := range accumulated {
		t.Logf("  %s: %d anomalies (first=%d, last=%d)", ac.Pattern, len(ac.Anomalies), ac.FirstSeen, ac.LastUpdated)
	}

	assert.NotEmpty(t, accumulated, "cluster should be accumulated before eviction")
	if len(accumulated) > 0 {
		assert.Equal(t, 2, len(accumulated[0].Anomalies), "cluster should contain both anomalies")
	}
}

// TestStepAdvance_MultipleTimestampGroupsFarBehindUpTo tests that clusters from
// multiple timestamp groups are accumulated even when all groups are >WindowSeconds
// behind upTo. This catches the stale currentDataTime issue: after a previous
// Advance(upTo), the correlator's currentDataTime is high, causing immediate
// eviction of historical-timestamp clusters.
func TestStepAdvance_MultipleTimestampGroupsFarBehindUpTo(t *testing.T) {
	e, _ := makeEngine([]observer.Anomaly{
		makeTestAnomaly("metric_a", 100),
		makeTestAnomaly("metric_b", 105),
	})

	// upTo=310, anomalies at 100 and 105 → both >120s behind.
	e.Advance(310)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated: %d correlations", len(accumulated))
	for _, ac := range accumulated {
		t.Logf("  %s: %d anomalies (first=%d, last=%d)", ac.Pattern, len(ac.Anomalies), ac.FirstSeen, ac.LastUpdated)
	}

	assert.NotEmpty(t, accumulated, "cluster should be accumulated before eviction")
}

// TestStepAdvance_SuccessiveAdvanceCallsPreserveClusters tests that clusters
// from one Advance call survive into the next. This simulates the live flow
// where the engine advances every second. The first advance detects anomalies
// at a historical timestamp, the second advance should still see the accumulated
// cluster.
func TestStepAdvance_SuccessiveAdvanceCallsPreserveClusters(t *testing.T) {
	storage := newTimeSeriesStorage()
	for sec := int64(0); sec < 400; sec++ {
		storage.Add("ns", "metric_a", 100.0, sec, nil)
		storage.Add("ns", "metric_b", 100.0, sec, nil)
	}

	// Detector returns anomalies on first call, nothing after.
	detector := &fixedDetector{
		anomalies: []observer.Anomaly{
			makeTestAnomaly("metric_a", 100),
			makeTestAnomaly("metric_b", 100),
		},
	}
	correlator := NewTimeClusterCorrelator(DefaultTimeClusterConfig())

	e := newEngine(engineConfig{
		storage:     storage,
		detectors:   []observer.Detector{detector},
		correlators: []observer.Correlator{correlator},
	})

	// First advance: detects anomalies at ts=100, upTo=310.
	e.Advance(310)

	acc1 := e.AccumulatedCorrelations()
	t.Logf("After first advance: %d correlations", len(acc1))
	assert.NotEmpty(t, acc1, "cluster should be accumulated on first advance")

	// Second advance: no new anomalies, upTo=311. Cluster should still be
	// in the accumulated map (not lost by stale currentDataTime).
	e.Advance(311)

	acc2 := e.AccumulatedCorrelations()
	t.Logf("After second advance: %d correlations", len(acc2))
	assert.NotEmpty(t, acc2, "cluster should persist in accumulated map across advances")
	assert.Equal(t, len(acc1), len(acc2), "no correlations should be lost between advances")
}

// TestStepAdvance_SingleGroupWithinWindow confirms clusters within WindowSeconds
// of upTo still work (baseline — should always pass).
func TestStepAdvance_SingleGroupWithinWindow(t *testing.T) {
	e, _ := makeEngine([]observer.Anomaly{
		makeTestAnomaly("metric_a", 100),
		makeTestAnomaly("metric_b", 100),
	})

	// upTo=200, anomalies at 100 → 100s gap, within 120s window.
	e.Advance(200)

	accumulated := e.AccumulatedCorrelations()
	t.Logf("Accumulated: %d correlations", len(accumulated))
	assert.NotEmpty(t, accumulated, "cluster within window should always be accumulated")
}
