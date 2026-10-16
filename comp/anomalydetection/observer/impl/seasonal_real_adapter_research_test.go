// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

import (
	"math"
	"reflect"
	"sort"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func realSeasonalTestFactory() observer.Detector {
	c := DefaultBOCPDConfig()
	c.WarmupPoints = 40
	c.Hazard = .05
	c.CPMassThreshold = .7
	c.Aggregations = []observer.Aggregate{observer.AggregateAverage, observer.AggregateCount}
	return NewBOCPDDetector(c)
}
func realSeasonalTestWrapper(mode string) *realSeasonalDetector {
	return &realSeasonalDetector{Detector: realSeasonalTestFactory(), factory: realSeasonalTestFactory, states: make(map[observer.SeriesRef]*realSeasonalGaugeState), mode: mode, budget: seasonalAdmissionFromEnvironment()}
}
func realSeasonalTestMeta(t *testing.T, storage observer.StorageReader) observer.SeriesMeta {
	t.Helper()
	xs := storage.ListSeries(observer.WorkloadSeriesFilter())
	if len(xs) != 1 {
		t.Fatalf("expected one test series, got%d", len(xs))
	}
	return xs[0]
}

func TestRealSeasonalClosedSlotCausality(t *testing.T) {
	m, err := newHourlyAdaptiveSeasonalModel(15, []int{1800, 3600})
	if err != nil {
		t.Fatal(err)
	}
	s := &realSeasonalGaugeState{model: m, pending: map[int64]float64{100: 10, 110: 30}}
	// Correcting an existing timestamp replaces its contribution, not its weight.
	s.pending[100] = 20
	ready, err := s.closeSlot()
	if err != nil || ready {
		t.Fatalf("unexpected readiness/error %v/%v", ready, err)
	}
	got, ok := m.sample(0)
	if !ok || got != 25 || m.Last != 0 || s.slotCounts[2] != 1 || s.maxWithinSlotSpread != 10 {
		t.Fatalf("closed mean/correction not respected: %+v", s)
	}
	// A missing slot does not extend or fill training.
	s.slot = 2
	s.pending = map[int64]float64{130: 40}
	if _, err = s.closeSlot(); err == nil || m.Last != 0 {
		t.Fatal("missing slot was accepted or model advanced")
	}
	s.slot = 1
	s.pending = map[int64]float64{115: math.NaN()}
	if _, err = s.closeSlot(); err == nil || m.Last != 0 {
		t.Fatal("nonfinite slot advanced training")
	}
}

func TestRealSeasonalCalibrationActualPointWeight(t *testing.T) {
	m := &hourlyAdaptiveSeasonalModel{Last: 1919, Profile: &hourlySeasonalProfile{Period: 120, Center: make([]float64, 120), Scale: make([]float64, 120)}}
	s := &realSeasonalGaugeState{model: m, frozen: true, closed: 1920, slot: 1920, pending: map[int64]float64{28801: 10, 28810: 30}}
	if ready, err := s.closeSlot(); ready || err != nil {
		t.Fatalf("unexpected readiness/error %v/%v", ready, err)
	}
	s.slot = 1921
	s.pending = map[int64]float64{28816: 80}
	if _, err := s.closeSlot(); err != nil {
		t.Fatal(err)
	}
	if m.Last != 1919 || !reflect.DeepEqual(s.calibration, []observer.Point{{Timestamp: 28801, Value: 10}, {Timestamp: 28810, Value: 30}, {Timestamp: 28816, Value: 80}}) {
		t.Fatalf("calibration must retain three actual observations, got%v", s.calibration)
	}
	v := realSeasonalVariance([]float64{s.calibration[0].Value, s.calibration[1].Value, s.calibration[2].Value})
	if math.Abs(v-1300) > 1e-10 {
		t.Fatalf("wrong weighted variance%g", v)
	}
}

func TestRealSeasonalReaderBoundsAndMatureBoundary(t *testing.T) {
	const origin int64 = 100000
	storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 5000})
	for i := 2279; i <= 2400; i++ {
		v := float64(i % 7)
		if i == 2400 {
			v = 1e12
		}
		storage.Add("contract", "redis.mem.used", v, origin+int64(i)*15, nil)
	}
	meta := realSeasonalTestMeta(t, storage)
	bounded := &realSeasonalReader{StorageReader: storage, ref: meta.Ref, minimum: origin + 2279*15, maximum: origin + 2399*15}
	if n := bounded.PointCountUpTo(meta.Ref, origin+2400*15); n != 120 {
		t.Fatalf("future visible or wrong replay count%d", n)
	}
	d := realSeasonalTestWrapper("phase_pooled_floor")
	m := &hourlyAdaptiveSeasonalModel{Last: 1919, Revision: 2, Offset: 3, Profile: &hourlySeasonalProfile{Period: 120, Center: make([]float64, 120), Scale: make([]float64, 120)}}
	scale := &realSeasonalScale{Original: make([]float64, 120), Applied: make([]float64, 120), V0: 9}
	for i := range scale.Applied {
		scale.Original[i] = 2
		scale.Applied[i] = 3
	}
	s := &realSeasonalGaugeState{model: m, scale: scale, detector: realSeasonalGaugeDetector(d.factory, observer.AggregateAverage, 0), countDetector: realSeasonalGaugeDetector(d.factory, observer.AggregateCount, 0), origin: origin, last: origin + 2399*15, started: true, slot: 2399, closed: 2399, pending: map[int64]float64{origin + 2399*15: float64(2399 % 7)}, frozen: true}
	expected := make([]float64, 480)
	for i := 1920; i < 2399; i++ {
		v := float64(i % 7)
		s.calibration = append(s.calibration, observer.Point{Timestamp: origin + int64(i)*15, Value: v})
		expected[i-1920] = v / 3
	}
	expected[479] = float64(2399%7) / 3
	d.states[meta.Ref] = s
	d.Detect(storage, origin+2400*15)
	if !s.mature || s.resets != 1 || m.Last != 1919 || m.Revision != 2 || s.closed != 2400 {
		t.Fatalf("wrong maturity/freeze: %+v", s)
	}
	if math.Abs(s.calibrationVariance-realSeasonalVariance(expected)) > 1e-12 {
		t.Fatal("arriving outlier leaked into calibration")
	}
	actual := s.detector.(*BOCPDDetector).series[bocpdStateKey{ref: meta.Ref, agg: observer.AggregateAverage}]
	if actual == nil || actual.lastProcessedTime != origin+2400*15 || actual.obsVar > 10 {
		t.Fatal("arriving outlier contaminated warmup variance or was not subsequently processed")
	}
	previous := s.detector
	storage.Add("contract", "redis.mem.used", 0, origin+2401*15, nil)
	d.Detect(storage, origin+2401*15)
	if s.resets != 1 || s.detector != previous || m.Last != 1919 {
		t.Fatal("second reset or post-training Observe")
	}
}

func realSeasonalSortAnomalies(a []observer.Anomaly) {
	sort.Slice(a, func(i, j int) bool { return a[i].SourceRef.Aggregate < a[j].SourceRef.Aggregate })
}
func TestRealSeasonalRawSplitAndFallbackParity(t *testing.T) {
	for _, mode := range []string{"raw_wrapped_nochange", "raw_fullcycle", "residual_fullcycle", "phase_pooled_floor"} {
		t.Run(mode, func(t *testing.T) {
			storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
			raw := realSeasonalTestFactory()
			wrapped := realSeasonalTestWrapper(mode)
			for i := 0; i < 200; i++ {
				// An early gap permanently disqualifies all mature modes; no reset/fill.
				ts := int64(100000 + 15*i)
				if i >= 5 {
					ts += 15
				}
				v := 100 + float64(i%9)
				if i >= 90 && i < 110 {
					v += 300
				}
				storage.Add("contract", "redis.mem.used", v, ts, nil)
				a, b := raw.Detect(storage, ts), wrapped.Detect(storage, ts)
				realSeasonalSortAnomalies(a.Anomalies)
				realSeasonalSortAnomalies(b.Anomalies)
				if !reflect.DeepEqual(a, b) || raw.Ready() != wrapped.Ready() {
					t.Fatalf("split/fallback changed output at%d: %#v / %#v", i, a, b)
				}
			}
			s := wrapped.states[realSeasonalTestMeta(t, storage).Ref]
			if s.reason != "missing_prereadiness_slot" || s.mature || s.resets != 0 {
				t.Fatalf("fallback not permanent: %+v", s)
			}
		})
	}
}

func TestRealSeasonalScaleMatchesHandCalculation(t *testing.T) {
	m := &hourlyAdaptiveSeasonalModel{Last: 1919, History: make([]float64, 1920), Valid: make([]bool, 1920), Profile: &hourlySeasonalProfile{Period: 120, Center: make([]float64, 120)}}
	for i := range m.History {
		m.Valid[i] = true
		if i%2 == 0 {
			m.History[i] = 10
		} else {
			m.History[i] = 20
		}
	}
	for p := range m.Profile.Center {
		if p%2 == 0 {
			m.Profile.Center[p] = 8
		} else {
			m.Profile.Center[p] = 20
		}
	}
	got, err := realSeasonalFitScale(m)
	if err != nil {
		t.Fatal(err)
	}
	v0 := 1.4826 * 1.4826
	if got.TrainingStart != 960 || math.Abs(got.V0-v0) > 1e-14 {
		t.Fatal("wrong recent training window or pooled variance")
	}
	for p, v := range got.Original {
		square := 0.0
		if p%2 == 0 {
			square = 32
		}
		want := math.Sqrt((square + 2*v0) / 10)
		if math.Abs(v-want) > 1e-14 || got.Applied[p] != math.Max(v, math.Sqrt(got.V0)) {
			t.Fatalf("wrong phase%d scale", p)
		}
	}
}

func TestRealSeasonalPendingStorageCorrection(t *testing.T) {
	storage := newTimeSeriesStorageWith(StorageConfig{MaxPointsPerSeries: 120})
	d := realSeasonalTestWrapper("raw_wrapped_nochange")
	storage.Add("contract", "redis.mem.used", 10, 100, nil)
	d.Detect(storage, 100)
	// Storage merges same-timestamp values into average20. Another timestamp in
	// the same slot contributes40. Training mean must be30, not25 or26.67.
	storage.Add("contract", "redis.mem.used", 30, 100, nil)
	storage.Add("contract", "redis.mem.used", 40, 110, nil)
	d.Detect(storage, 110)
	storage.Add("contract", "redis.mem.used", 999, 115, nil)
	d.Detect(storage, 115)
	s := d.states[realSeasonalTestMeta(t, storage).Ref]
	v, ok := s.model.sample(0)
	if !ok || v != 30 || s.model.Last != 0 || s.slotCounts[2] != 1 {
		t.Fatalf("pending correction or closed-slot causality violated: value%g last%d", v, s.model.Last)
	}
}
