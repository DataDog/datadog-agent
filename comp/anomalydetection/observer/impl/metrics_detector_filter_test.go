// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

func filterTestDetectors() map[string]func() observer.Detector {
	return map[string]func() observer.Detector{
		"bocpd": func() observer.Detector {
			cfg := DefaultBOCPDConfig()
			cfg.WarmupPoints, cfg.MaxRunLength = 4, 24
			return NewBOCPDDetector(cfg)
		},
		"scanmw": func() observer.Detector {
			cfg := DefaultScanMWConfig()
			cfg.MinPoints, cfg.MinSegment, cfg.MaxPoints = 6, 2, 24
			return NewScanMWDetector(cfg)
		},
		"scanwelch": func() observer.Detector {
			dCfg := DefaultScanWelchConfig()
			dCfg.MinPoints, dCfg.MinSegment, dCfg.MaxPoints = 6, 2, 24
			d := NewScanWelchDetector(dCfg)
			return d
		},
		"holt_residual": func() observer.Detector {
			dCfg := DefaultHoltResidualConfig()
			dCfg.WarmupPoints, dCfg.ResidualWindow = 4, 8
			d := NewHoltResidualDetector(dCfg)
			return d
		},
		"tukey_biweight": func() observer.Detector {
			dCfg := DefaultTukeyBiweightConfig()
			dCfg.MinPoints, dCfg.WindowSize, dCfg.ScoreEvery = 6, 8, 1
			d := NewTukeyBiweightDetector(dCfg)
			return d
		},
		"adapter": func() observer.Detector {
			return newSeriesDetectorAdapter(&filterSeriesDetector{}, defaultAggregations)
		},
	}
}

type filterSeriesDetector struct{ ready bool }

func (*filterSeriesDetector) Name() string  { return "filter_series" }
func (d *filterSeriesDetector) Ready() bool { return d.ready }
func (d *filterSeriesDetector) Reset()      { d.ready = false }
func (d *filterSeriesDetector) Detect(series observer.Series) observer.DetectionResult {
	d.ready = true
	return observer.DetectionResult{Anomalies: []observer.Anomaly{{Timestamp: series.Points[len(series.Points)-1].Timestamp}}}
}

// Snapshot values, not state pointers, to detect changes to excluded state.
func filterTestState(detector observer.Detector) map[observer.QueryHandle]string {
	states := make(map[observer.QueryHandle]string)
	add := func(ref observer.SeriesRef, agg observer.Aggregate, state any) {
		states[observer.QueryHandle{Ref: ref, Aggregate: agg}] = fmt.Sprintf("%#v", state)
	}
	switch d := detector.(type) {
	case *BOCPDDetector:
		for key, state := range d.series {
			add(key.ref, key.agg, *state)
		}
	case *ScanMWDetector:
		for key, state := range d.series {
			add(key.ref, key.agg, *state)
		}
	case *ScanWelchDetector:
		for key, state := range d.series {
			add(key.ref, key.agg, *state)
		}
	case *HoltResidualDetector:
		for key, state := range d.series {
			add(key.ref, key.agg, *state)
		}
	case *TukeyBiweightDetector:
		for key, state := range d.series {
			add(key.ref, key.agg, *state)
		}
	case *seriesDetectorAdapter:
		for key, count := range d.lastVisibleCount {
			add(key.Ref, key.Aggregate, count)
		}
	default:
		panic(fmt.Sprintf("unhandled detector %T", detector))
	}
	return states
}

type filterTestStorage struct {
	*timeSeriesStorage
	listCalls   int
	averageOnly bool
}

func (s *filterTestStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesMeta {
	s.listCalls++
	return s.timeSeriesStorage.ListSeries(filter)
}
func (s *filterTestStorage) ListSeriesRefsInto(filter observer.SeriesFilter, dst []observer.SeriesRef) []observer.SeriesRef {
	s.listCalls++
	return s.timeSeriesStorage.ListSeriesRefsInto(filter, dst)
}
func (s *filterTestStorage) SupportsAggregate(ref observer.SeriesRef, agg observer.Aggregate) bool {
	return (!s.averageOnly || agg == observer.AggregateAverage) && s.timeSeriesStorage.SupportsAggregate(ref, agg)
}

func addFilterTestSeries(s *filterTestStorage, key uint64, namespace string, mt metrics.MetricType, kind observer.MetricKind) observer.SeriesRef {
	var ref observer.SeriesRef
	for ts := int64(1); ts <= 20; ts++ {
		ref = s.AddWithKeyAndHostComposite(namespace, fmt.Sprintf("metric_%d", key), "", 100, ts, tagset.CompositeTags{}, key, mt).Ref
	}
	s.SetSeriesKind(ref, kind)
	return ref
}

func newFilterTestStorage() (*filterTestStorage, []observer.SeriesRef) {
	s := &filterTestStorage{timeSeriesStorage: newTimeSeriesStorage()}
	refs := []observer.SeriesRef{
		addFilterTestSeries(s, 1, "work", metrics.GaugeType, observer.STABLE),
		addFilterTestSeries(s, 2, "work", metrics.GaugeType, observer.LINEAR),
		addFilterTestSeries(s, 3, "work", metrics.CountType, observer.STABLE),
		addFilterTestSeries(s, 4, "work", observer.UnknownType, observer.UNSET),
	}
	addFilterTestSeries(s, 5, observer.TelemetryNamespace, metrics.GaugeType, observer.STABLE)
	return s, refs
}

func TestDetectorFilterSelection(t *testing.T) {
	stable, linear, unset := observer.STABLE, observer.LINEAR, observer.UNSET
	gauge, count, unknownType := metrics.GaugeType, metrics.CountType, observer.UnknownType
	avg, cnt, sum := observer.AggregateAverage, observer.AggregateCount, observer.AggregateSum
	for name, factory := range filterTestDetectors() {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				filter      observer.DetectionFilter
				indices     []int
				aggs        []observer.Aggregate
				averageOnly bool
			}{
				{"zero", observer.DetectionFilter{}, []int{0, 1, 2, 3}, []observer.Aggregate{avg, cnt}, false},
				{"kind", observer.DetectionFilter{MetricKind: &stable}, []int{0, 2}, []observer.Aggregate{avg, cnt}, false},
				{"type", observer.DetectionFilter{MetricType: &gauge}, []int{0, 1}, []observer.Aggregate{avg, cnt}, false},
				{"combined", observer.DetectionFilter{MetricKind: &stable, MetricType: &count}, []int{2}, []observer.Aggregate{avg, cnt}, false},
				{"unset", observer.DetectionFilter{MetricKind: &unset}, []int{3}, []observer.Aggregate{avg, cnt}, false},
				{"unknown type", observer.DetectionFilter{MetricType: &unknownType}, []int{3}, []observer.Aggregate{avg, cnt}, false},
				{"no series", observer.DetectionFilter{MetricKind: &linear, MetricType: &count}, nil, nil, false},
				{"intersection", observer.DetectionFilter{Aggregations: []observer.Aggregate{sum, avg, avg}}, []int{0, 1, 2, 3}, []observer.Aggregate{avg}, false},
				{"no aggregation", observer.DetectionFilter{Aggregations: []observer.Aggregate{sum}}, nil, nil, false},
				{"order and duplicates", observer.DetectionFilter{MetricKind: &linear, Aggregations: []observer.Aggregate{cnt, avg, cnt}}, []int{1}, []observer.Aggregate{avg, cnt}, false},
				{"unsupported aggregate", observer.DetectionFilter{Aggregations: []observer.Aggregate{cnt}}, nil, nil, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					s, refs := newFilterTestStorage()
					s.averageOnly = tc.averageOnly
					d := factory()
					result := d.Detect(s, 20, tc.filter)
					var want, got []observer.QueryHandle
					for _, index := range tc.indices {
						for _, agg := range tc.aggs {
							want = append(want, observer.QueryHandle{Ref: refs[index], Aggregate: agg})
						}
					}
					for key := range filterTestState(d) {
						got = append(got, key)
					}
					require.ElementsMatch(t, want, got)
					for _, anomaly := range result.Anomalies {
						require.Contains(t, want, *anomaly.SourceRef)
					}
					if name == "adapter" {
						require.Len(t, result.Anomalies, len(want), "one call per eligible aggregation")
						if len(tc.indices) == 1 {
							for i, anomaly := range result.Anomalies {
								require.Equal(t, want[i], *anomaly.SourceRef)
							}
						}
					}
					require.Equal(t, int64(100), s.TotalSampleCount(""), "filtering must retain history")
				})
			}
		})
	}
}

func TestDetectorFilterConstantSelectionAndFreshness(t *testing.T) {
	for name, factory := range filterTestDetectors() {
		t.Run(name, func(t *testing.T) {
			s, refs := newFilterTestStorage()
			d := factory()
			kind, mt := observer.STABLE, metrics.GaugeType
			filter := observer.DetectionFilter{MetricKind: &kind, MetricType: &mt, Aggregations: []observer.Aggregate{observer.AggregateAverage}}
			d.Detect(s, 20, filter)
			require.Len(t, filterTestState(d), 1)
			require.True(t, d.Ready())
			calls := s.listCalls
			// Equal values and aggregation-only changes reuse the series selection.
			otherKind, otherType := kind, mt
			filter.MetricKind, filter.MetricType = &otherKind, &otherType
			d.Detect(s, 20, filter)
			require.Equal(t, calls, s.listCalls, "equal restrictions must reuse selection")
			filter.Aggregations = []observer.Aggregate{observer.AggregateCount}
			result := d.Detect(s, 20, filter)
			require.Equal(t, calls, s.listCalls)
			selected := observer.QueryHandle{Ref: refs[0], Aggregate: observer.AggregateCount}
			require.Contains(t, filterTestState(d), selected, "count remains fresh after average-only processing")
			if name == "adapter" {
				require.Len(t, result.Anomalies, 1)
				require.Equal(t, selected, *result.Anomalies[0].SourceRef)
			}
			before := filterTestState(d)
			for i, ref := range refs {
				meta := s.GetSeriesMeta(ref)
				s.AddWithKeyAndHostComposite(meta.Namespace, meta.Name, meta.Host, 110, 21, meta.Tags, uint64(i+1), meta.MetricType)
			}
			d.Detect(s, 21, filter)
			require.Equal(t, calls, s.listCalls, "point writes must not invalidate selection")
			after := filterTestState(d)
			for key, state := range before {
				if key != selected {
					require.Equal(t, state, after[key], "excluded state advanced: %v", key)
				}
			}
			require.NotEqual(t, before[selected], after[selected])
			filter.Aggregations = []observer.Aggregate{observer.AggregateSum}
			d.Detect(s, 21, filter)
			require.Equal(t, after, filterTestState(d))
			require.True(t, d.Ready(), "filtering does not clear readiness")
		})
	}
}

func TestDetectorFilterMetadataAndCleanup(t *testing.T) {
	for name, factory := range filterTestDetectors() {
		t.Run(name, func(t *testing.T) {
			s, refs := newFilterTestStorage()
			d := factory()
			kind, mt := observer.STABLE, metrics.GaugeType
			filter := observer.DetectionFilter{MetricKind: &kind, MetricType: &mt}
			s.SetSeriesKind(refs[3], observer.STABLE)
			d.Detect(s, 20, filter)
			calls := s.listCalls
			s.SetSeriesKind(refs[1], observer.STABLE)
			d.Detect(s, 20, filter)
			require.Equal(t, calls+1, s.listCalls)
			s.AddWithKeyAndHostComposite("work", "metric_4", "", 100, 20, tagset.CompositeTags{}, 4, metrics.GaugeType)
			d.Detect(s, 20, filter)
			require.Equal(t, calls+2, s.listCalls)
			newRef := addFilterTestSeries(s, 6, "work", metrics.GaugeType, observer.STABLE)
			d.Detect(s, 20, filter)
			require.Equal(t, calls+3, s.listCalls)
			require.Len(t, filterTestState(d), 8, "newly eligible series must be processed")
			before := filterTestState(d)
			s.SetSeriesKind(refs[0], observer.LINEAR)
			s.AddWithKeyAndHostComposite("work", "metric_1", "", 110, 21, tagset.CompositeTags{}, 1, metrics.GaugeType)
			d.Detect(s, 21, filter)
			require.Equal(t, calls+4, s.listCalls, "kind changes invalidate selection")
			require.Equal(t, before, filterTestState(d), "kind-excluded state must not advance with fresh data")

			d.(observer.SeriesRemover).RemoveSeries([]observer.SeriesRef{newRef})
			for key := range filterTestState(d) {
				require.NotEqual(t, newRef, key.Ref)
			}
			require.Equal(t, 20, s.PointCountUpTo(newRef, 20), "detector cleanup retains storage history")
			d.Detect(s, 20, filter)
			require.Len(t, filterTestState(d), 8, "reactivation can reuse stored observations")
			d.(interface{ Reset() }).Reset()
			require.Empty(t, filterTestState(d))
			require.False(t, d.Ready())
			d.Detect(s, 20, filter)
			require.Len(t, filterTestState(d), 6, "reset must rebuild the current selection")
			calls = s.listCalls
			s.RemoveSeriesByRefs([]observer.SeriesRef{newRef})
			d.Detect(s, 21, filter)
			require.Equal(t, calls+1, s.listCalls, "storage removal invalidates selection")
			d.Detect(s, 21, filter)
			require.Equal(t, calls+1, s.listCalls, "unchanged membership reuses selection")
			d.(observer.SeriesRemover).RemoveSeries([]observer.SeriesRef{newRef})
			require.Len(t, filterTestState(d), 4)
		})
	}
}

func TestDetectorFilterOutputsMatchUnfiltered(t *testing.T) {
	for _, entry := range defaultCatalog().Entries() {
		if entry.kind != componentDetector {
			continue
		}
		t.Run(entry.name, func(t *testing.T) {
			scoped := entry.factory(entry.defaultConfig).(observer.Detector)
			unfiltered := entry.factory(entry.defaultConfig).(observer.Detector)
			replayed := entry.factory(entry.defaultConfig).(observer.Detector)
			s := newTimeSeriesStorage()
			preloaded := newDetectorTestStorage()
			kind, mt := observer.STABLE, metrics.GaugeType
			filter := observer.DetectionFilter{MetricKind: &kind, MetricType: &mt, Aggregations: []observer.Aggregate{observer.AggregateAverage}}
			valueAt := func(i int64) float64 { return float64(100 + (i/101)%3*20 + i%3) }
			for i := int64(1); i <= 500; i++ {
				ref := preloaded.AddWithKeyAndHostComposite("work", "metric", "", valueAt(i), i, tagset.CompositeTags{}, 1, metrics.GaugeType).Ref
				preloaded.SetSeriesKind(ref, kind)
			}
			found := 0
			for i := int64(1); i <= 500; i++ {
				ref := s.AddWithKeyAndHostComposite("work", "metric", "", valueAt(i), i, tagset.CompositeTags{}, 1, metrics.GaugeType).Ref
				s.SetSeriesKind(ref, kind)
				want := unfiltered.Detect(s, i, observer.DetectionFilter{}).Anomalies
				got := scoped.Detect(s, i, filter).Anomalies
				require.ElementsMatch(t, want, got, "at observation %d", i)
				require.ElementsMatch(t, got, replayed.Detect(preloaded, i, filter).Anomalies, "replay at observation %d", i)
				require.Empty(t, scoped.Detect(s, i, filter).Anomalies, "unchanged advances must not reemit")
				found += len(got)
			}
			require.Positive(t, found, "fixture must exercise anomaly output")
		})
	}
}

func TestSeriesDetectorAdapterConfigOwnership(t *testing.T) {
	aggregations := []observer.Aggregate{observer.AggregateAverage}
	d := newSeriesDetectorAdapter(&filterSeriesDetector{}, aggregations)
	aggregations[0] = observer.AggregateSum
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage}, d.aggregations)
	d.Reset()
	require.Equal(t, []observer.Aggregate{observer.AggregateAverage}, d.aggregations)
}
