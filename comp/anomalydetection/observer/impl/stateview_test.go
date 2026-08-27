// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sync"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func TestStateView_StorageAccess(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)
	storage.Add("ns", "cpu", 2.0, 101, nil)
	storage.Add("ns", "mem", 512.0, 100, []string{"host:a"})

	e := newEngine(engineConfig{storage: storage})
	sv := e.StateView()

	// ListSeries
	keys := sv.ListSeries(observerdef.SeriesFilter{Namespace: "ns"})
	if len(keys) != 2 {
		t.Fatalf("expected 2 series, got %d", len(keys))
	}

	// GetSeriesRange — find the "cpu" series ID from ListSeries
	cpuHandle := observerdef.SeriesRef(-1)
	for _, m := range keys {
		if m.Name == "cpu" {
			cpuHandle = m.Ref
		}
	}
	series := sv.GetSeriesRange(cpuHandle, 0, 200, observerdef.AggregateAverage)
	if series == nil {
		t.Fatal("expected series data, got nil")
	}
	if len(series.Points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(series.Points))
	}

	// ScenarioBounds
	start, end, ok := sv.ScenarioBounds()
	if !ok {
		t.Fatal("expected bounds to be available")
	}
	if start != 100 || end != 101 {
		t.Fatalf("expected bounds [100, 101], got [%d, %d]", start, end)
	}
}

func TestStateView_Anomalies(t *testing.T) {
	e := newEngine(engineConfig{
		storage:             newTimeSeriesStorage(),
		trackAnomalyHistory: true,
	})
	sv := e.StateView()

	// Initially empty
	if len(sv.Anomalies()) != 0 {
		t.Fatalf("expected 0 anomalies, got %d", len(sv.Anomalies()))
	}
	if sv.TotalAnomalyCount() != 0 {
		t.Fatalf("expected 0 total anomalies, got %d", sv.TotalAnomalyCount())
	}

	// Add some anomalies via the engine
	e.acceptAnomaly(observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: "cpu"},
		DetectorName: "detector_a",
		Timestamp:    100,
	}, 100)
	e.acceptAnomaly(observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: "mem"},
		DetectorName: "bocpd",
		Timestamp:    101,
	}, 101)

	if len(sv.Anomalies()) != 2 {
		t.Fatalf("expected 2 anomalies, got %d", len(sv.Anomalies()))
	}
	if sv.TotalAnomalyCount() != 2 {
		t.Fatalf("expected 2 total anomalies, got %d", sv.TotalAnomalyCount())
	}
	if sv.UniqueAnomalySourceCount() != 2 {
		t.Fatalf("expected 2 unique sources, got %d", sv.UniqueAnomalySourceCount())
	}

	// DetectorAnomalies filters correctly
	detectorAAnomalies := sv.DetectorAnomalies("detector_a")
	if len(detectorAAnomalies) != 1 {
		t.Fatalf("expected 1 detector_a anomaly, got %d", len(detectorAAnomalies))
	}
	if detectorAAnomalies[0].DetectorName != "detector_a" {
		t.Fatalf("expected detector_a, got %s", detectorAAnomalies[0].DetectorName)
	}

	// AnomaliesByDetector groups correctly
	byDetector := sv.AnomaliesByDetector()
	if len(byDetector) != 2 {
		t.Fatalf("expected 2 detector groups, got %d", len(byDetector))
	}
	if len(byDetector["detector_a"]) != 1 {
		t.Fatalf("expected 1 detector_a anomaly, got %d", len(byDetector["detector_a"]))
	}
	if len(byDetector["bocpd"]) != 1 {
		t.Fatalf("expected 1 bocpd anomaly, got %d", len(byDetector["bocpd"]))
	}

	// AnomaliesForSource filters by SeriesDescriptor
	diskDesc := observerdef.SeriesDescriptor{Name: "disk", Aggregate: observerdef.AggregateAverage}
	e.acceptAnomaly(observerdef.Anomaly{
		Source:       diskDesc,
		DetectorName: "detector_a",
		Timestamp:    102,
	}, 102)
	diskAnomalies := sv.AnomaliesForSource(diskDesc)
	if len(diskAnomalies) != 1 {
		t.Fatalf("expected 1 disk anomaly, got %d", len(diskAnomalies))
	}
	if diskAnomalies[0].Source.Name != "disk" {
		t.Fatalf("expected disk source, got %s", diskAnomalies[0].Source.Name)
	}
	// Matching by name should find the correct anomaly
	cpuAnomalies := sv.AnomaliesForSource(observerdef.SeriesDescriptor{Name: "cpu"})
	if len(cpuAnomalies) != 1 {
		t.Fatalf("expected 1 cpu anomaly, got %d", len(cpuAnomalies))
	}
	if cpuAnomalies[0].Source.Name != "cpu" {
		t.Fatalf("expected cpu source, got %s", cpuAnomalies[0].Source.Name)
	}
}

func TestLiveAnomalyTrackingDeduplicatesWithoutRetainingHistory(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	anomaly := observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: "cpu"},
		DetectorName: "detector_a",
		Title:        "spike",
		Timestamp:    100,
	}

	if !e.acceptAnomaly(anomaly, 100) {
		t.Fatal("expected the first anomaly to be accepted")
	}
	if e.acceptAnomaly(anomaly, 101) {
		t.Fatal("expected the repeated anomaly to be deduplicated")
	}
	if got := len(e.RawAnomalies()); got != 0 {
		t.Fatalf("live mode retained %d raw anomalies, expected none", got)
	}
	if got := len(e.uniqueAnomalySources); got != 0 {
		t.Fatalf("live mode retained %d unique anomaly sources, expected none", got)
	}
	if got := len(e.anomalyDeduper.entries); got != 1 {
		t.Fatalf("live dedup cache has %d entries, expected 1", got)
	}
}

func TestAnomalyDeduperBoundsEntries(t *testing.T) {
	key := func(source string, timestamp int64) anomalyDedupKey {
		return anomalyDedupKey{sourceKey: source, detectorName: "detector", timestamp: timestamp}
	}

	t.Run("data time", func(t *testing.T) {
		deduper := newAnomalyDeduper(10, 10)
		first := key("first", 100)
		if !deduper.accept(first, 100) {
			t.Fatal("expected first key to be accepted")
		}
		if deduper.accept(first, 110) {
			t.Fatal("expected key at the retention boundary to remain deduplicated")
		}
		result := deduper.acceptWithEvictions(key("second", 111), 111)
		if !result.accepted {
			t.Fatal("expected second key to be accepted")
		}
		if result.ageEvicted != 1 || result.capacityEvicted != 0 {
			t.Fatalf("unexpected eviction counts: %+v", result)
		}
		if !deduper.accept(first, 111) {
			t.Fatal("expected expired key to be accepted again")
		}
	})

	t.Run("cardinality", func(t *testing.T) {
		deduper := newAnomalyDeduper(0, 2)
		first := key("first", 100)
		if !deduper.accept(first, 100) || !deduper.accept(key("second", 101), 101) {
			t.Fatal("expected unique keys to be accepted")
		}
		result := deduper.acceptWithEvictions(key("third", 102), 102)
		if !result.accepted || result.capacityEvicted != 1 || result.ageEvicted != 0 {
			t.Fatalf("unexpected capacity eviction result: %+v", result)
		}
		if got := len(deduper.entries); got != 2 {
			t.Fatalf("dedup cache has %d entries, expected cardinality cap 2", got)
		}
		if !deduper.accept(first, 102) {
			t.Fatal("expected the oldest capacity-evicted key to be accepted again")
		}
	})
}

func TestAnomalyDeduperRemovesStorageRefs(t *testing.T) {
	deduper := newAnomalyDeduper(100, 10)
	refKey := func(ref observerdef.SeriesRef, agg observerdef.Aggregate) anomalyDedupKey {
		return anomalyDedupKey{
			sourceRef:       ref,
			sourceAggregate: agg,
			hasSourceRef:    true,
			detectorName:    "detector",
			timestamp:       100,
		}
	}
	refOneAvg := refKey(1, observerdef.AggregateAverage)
	refOneSum := refKey(1, observerdef.AggregateSum)
	refTwo := refKey(2, observerdef.AggregateAverage)
	withoutRef := anomalyDedupKey{sourceKey: "rrcf/score", detectorName: "rrcf", timestamp: 100}
	for _, key := range []anomalyDedupKey{refOneAvg, refOneSum, refTwo, withoutRef} {
		if !deduper.accept(key, 100) {
			t.Fatalf("expected key to be accepted: %+v", key)
		}
	}

	if removed := deduper.removeSourceRefs([]observerdef.SeriesRef{1}); removed != 2 {
		t.Fatalf("removed %d dedup entries for ref 1, expected 2", removed)
	}
	if got := len(deduper.entries); got != 2 {
		t.Fatalf("dedup cache has %d entries after source removal, expected 2", got)
	}
	if _, exists := deduper.refHeads[1]; exists {
		t.Fatal("source-ref index retained an evicted ref")
	}
	if !deduper.accept(refOneAvg, 100) {
		t.Fatal("expected a removed source-ref key to be accepted again")
	}
	if deduper.accept(refTwo, 100) || deduper.accept(withoutRef, 100) {
		t.Fatal("removing ref 1 also removed unrelated dedup keys")
	}
}

func TestAnomalyDeduperRebuildsSourceIndexesAfterCompaction(t *testing.T) {
	const entryCount = 2048
	deduper := newAnomalyDeduper(0, entryCount)
	for i := 0; i < entryCount; i++ {
		key := anomalyDedupKey{
			sourceRef:    observerdef.SeriesRef(i),
			hasSourceRef: true,
			detectorName: "detector",
			timestamp:    100,
		}
		if !deduper.accept(key, 100) {
			t.Fatalf("expected ref %d to be accepted", i)
		}
	}

	refs := make([]observerdef.SeriesRef, entryCount/2)
	for i := range refs {
		refs[i] = observerdef.SeriesRef(i)
	}
	if removed := deduper.removeSourceRefs(refs); removed != len(refs) {
		t.Fatalf("removed %d entries before compaction, expected %d", removed, len(refs))
	}
	if len(deduper.queue) != entryCount/2 || deduper.inactive != 0 {
		t.Fatalf("queue was not compacted: len=%d inactive=%d", len(deduper.queue), deduper.inactive)
	}
	if removed := deduper.removeSourceRefs([]observerdef.SeriesRef{entryCount - 1}); removed != 1 {
		t.Fatalf("rebuilt source index removed %d entries, expected 1", removed)
	}
}

func TestLiveAnomalyDedupEvictionTelemetry(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	e.anomalyDeduper.maxAgeSecs = 10
	e.anomalyDeduper.maxEntries = 2
	evicted := make(map[string]int)
	e.onAnomalyDedupEvicted = func(reason string, count int) {
		evicted[reason] += count
	}
	anomaly := func(name string, timestamp int64) observerdef.Anomaly {
		return observerdef.Anomaly{
			Source:       observerdef.SeriesDescriptor{Name: name},
			DetectorName: "detector",
			Timestamp:    timestamp,
		}
	}

	e.acceptAnomaly(anomaly("first", 100), 100)
	e.acceptAnomaly(anomaly("second", 101), 101)
	e.acceptAnomaly(anomaly("third", 102), 102)
	e.acceptAnomaly(anomaly("fourth", 113), 113)

	ref := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage}
	withRef := anomaly("storage-backed", 114)
	withRef.SourceRef = &ref
	e.acceptAnomaly(withRef, 114)
	e.fanOutSeriesRemoval([]observerdef.SeriesRef{ref.Ref})

	if evicted[anomalyDedupEvictionReasonCapacity] != 1 {
		t.Fatalf("capacity eviction telemetry = %d, expected 1", evicted[anomalyDedupEvictionReasonCapacity])
	}
	if evicted[anomalyDedupEvictionReasonAge] != 2 {
		t.Fatalf("age eviction telemetry = %d, expected 2", evicted[anomalyDedupEvictionReasonAge])
	}
	if evicted[anomalyDedupEvictionReasonSeries] != 1 {
		t.Fatalf("series eviction telemetry = %d, expected 1", evicted[anomalyDedupEvictionReasonSeries])
	}
}

func TestAnomalyDedupKeyUsesStorageRefWhenAvailable(t *testing.T) {
	ref := observerdef.QueryHandle{Ref: 42, Aggregate: observerdef.AggregateAverage}
	key := anomalyDedupKeyFor(observerdef.Anomaly{
		Source: observerdef.SeriesDescriptor{
			Namespace: "dogstatsd",
			Name:      "cpu.user",
			Tags:      []string{"host:a"},
		},
		SourceRef:    &ref,
		DetectorName: "bocpd",
		Timestamp:    100,
	})

	if !key.hasSourceRef || key.sourceRef != ref.Ref || key.sourceAggregate != ref.Aggregate {
		t.Fatalf("dedup key did not preserve compact storage identity: %+v", key)
	}
	if key.sourceKey != "" {
		t.Fatalf("storage-backed anomaly allocated a string source key: %q", key.sourceKey)
	}
}

func TestResetForReplayConfiguresAnomalyHistory(t *testing.T) {
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	storageCfg := DefaultStorageConfig()
	storageCfg.TrackAnomalyHistory = true
	e.ResetForReplay(nil, nil, nil, nil, storageCfg, BaselineConfig{})

	anomaly := observerdef.Anomaly{
		Source:       observerdef.SeriesDescriptor{Name: "cpu"},
		DetectorName: "detector_a",
		Timestamp:    100,
	}
	if !e.acceptAnomaly(anomaly, 100) {
		t.Fatal("expected replay anomaly to be accepted")
	}
	if got := len(e.RawAnomalies()); got != 1 {
		t.Fatalf("replay mode retained %d raw anomalies, expected 1", got)
	}

	storageCfg.TrackAnomalyHistory = false
	e.ResetForReplay(nil, nil, nil, nil, storageCfg, BaselineConfig{})
	if !e.acceptAnomaly(anomaly, 100) {
		t.Fatal("expected live-mode anomaly after reset to be accepted")
	}
	if got := len(e.RawAnomalies()); got != 0 {
		t.Fatalf("live mode retained %d raw anomalies after reset, expected none", got)
	}
	if e.anomalyDeduper.maxAgeSecs <= 0 || e.anomalyDeduper.maxEntries <= 0 {
		t.Fatal("live mode dedup cache must be bounded by both age and cardinality")
	}
}

func TestStateView_DetectorsAndCorrelators(t *testing.T) {
	detector := &mockDetector{name: "mock_det"}
	correlator := &mockCorrelator{name: "mock_corr"}

	e := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		detectors:   []observerdef.Detector{detector},
		correlators: []observerdef.Correlator{correlator},
	})
	sv := e.StateView()

	detectors := sv.ListDetectors()
	if len(detectors) != 1 || detectors[0].Name != "mock_det" {
		t.Fatalf("unexpected detectors: %+v", detectors)
	}

	correlators := sv.ListCorrelators()
	if len(correlators) != 1 || correlators[0].Name != "mock_corr" {
		t.Fatalf("unexpected correlators: %+v", correlators)
	}
}

func TestStateView_SchedulingState(t *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 100, nil)
	storage.Add("ns", "cpu", 2.0, 200, nil)

	e := newEngine(engineConfig{storage: storage})
	sv := e.StateView()

	if sv.LastAnalyzedTime() != 0 {
		t.Fatalf("expected 0, got %d", sv.LastAnalyzedTime())
	}

	e.Advance(150)
	if sv.LastAnalyzedTime() != 150 {
		t.Fatalf("expected 150, got %d", sv.LastAnalyzedTime())
	}
}

// mockDetector is a minimal Detector for testing.
type mockDetector struct {
	name string
}

func (d *mockDetector) Name() string { return d.name }
func (*mockDetector) Ready() bool    { return true }
func (d *mockDetector) Detect(_ observerdef.StorageReader, _ int64) observerdef.DetectionResult {
	return observerdef.DetectionResult{}
}

// mockCorrelator is a minimal Correlator for testing.
type mockCorrelator struct {
	name string
}

func (c *mockCorrelator) Name() string                                        { return c.name }
func (c *mockCorrelator) ProcessAnomaly(_ observerdef.Anomaly)                {}
func (c *mockCorrelator) Advance(_ int64)                                     {}
func (c *mockCorrelator) PendingEvents() []observerdef.CorrelatorEvent        { return nil }
func (c *mockCorrelator) ActiveCorrelations() []observerdef.ActiveCorrelation { return nil }
func (c *mockCorrelator) Reset()                                              {}

func TestFindingM11_StateViewListDetectorsRace(_ *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:   storage,
		detectors: []observerdef.Detector{&mockDetector{name: "det1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetDetectors([]observerdef.Detector{
				&mockDetector{name: fmt.Sprintf("det_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ListDetectors()
		}
	}()

	wg.Wait()
}

func TestFindingM11_StateViewListCorrelatorsRace(_ *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "corr1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ListCorrelators()
		}
	}()

	wg.Wait()
}

func TestFindingM11_StateViewActiveCorrelationsRace(_ *testing.T) {
	storage := newTimeSeriesStorage()
	storage.Add("ns", "cpu", 1.0, 1, nil)

	e := newEngine(engineConfig{
		storage:     storage,
		correlators: []observerdef.Correlator{&mockCorrelator{name: "corr1"}},
	})
	sv := e.StateView()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.SetCorrelators([]observerdef.Correlator{
				&mockCorrelator{name: fmt.Sprintf("corr_%d", i)},
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = sv.ActiveCorrelations()
		}
	}()

	wg.Wait()
}
