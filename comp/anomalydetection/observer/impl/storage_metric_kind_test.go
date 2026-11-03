// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/require"
)

func TestStorageMetricKinds(t *testing.T) {
	s := newTimeSeriesStorage()
	s.ResetSeriesKinds()
	require.Zero(t, s.SeriesGeneration(), "empty reset must not invalidate")
	// Use distinct explicit keys: the storage API expects a resolved identity.
	ref := s.AddWithKeyAndHostComposite("check", "gauge", "", 10, 1, tagset.CompositeTags{}, 1, observer.UnknownType).Ref
	require.Equal(t, uint64(1), s.SeriesGeneration())
	generation := s.SeriesGeneration()
	require.Equal(t, observer.UNSET, s.GetSeriesMeta(ref).MetricKind)
	kind, ok := s.SeriesKind(ref)
	require.True(t, ok)
	require.Equal(t, observer.UNSET, kind)
	s.AddWithKeyAndHostComposite("check", "gauge", "", 11, 2, tagset.CompositeTags{}, 1, metrics.GaugeType)
	require.Equal(t, generation+1, s.SeriesGeneration())
	generation = s.SeriesGeneration()
	s.AddWithKeyAndHostComposite("check", "gauge", "", 12, 3, tagset.CompositeTags{}, 1, metrics.GaugeType)
	require.Equal(t, generation, s.SeriesGeneration())

	stable, gauge := observer.STABLE, metrics.GaugeType
	filter := observer.WorkloadSeriesFilter()
	filter.MetricType, filter.MetricKind = &gauge, &stable
	require.Empty(t, s.ListSeriesRefsInto(filter, nil))
	history := s.GetSeriesRange(ref, 0, 3, observer.AggregateAverage)
	writes := s.WriteGeneration(ref)
	s.SetSeriesKind(ref, stable)
	require.Equal(t, writes, s.WriteGeneration(ref))
	require.Equal(t, history, s.GetSeriesRange(ref, 0, 3, observer.AggregateAverage))
	require.Equal(t, stable, s.GetSeriesMeta(ref).MetricKind)
	require.Equal(t, generation+1, s.SeriesGeneration())
	require.Equal(t, []observer.SeriesRef{ref}, s.ListSeriesRefsInto(filter, nil))
	require.Equal(t, stable, s.ListSeries(filter)[0].MetricKind)
	require.Equal(t, stable, s.ListSeriesMetadata("check")[0].MetricKind)
	generation = s.SeriesGeneration()
	s.SetSeriesKind(ref, stable)
	s.SetSeriesKind(999, stable)
	require.Equal(t, generation, s.SeriesGeneration())

	second := s.AddWithKeyAndHostComposite("check", "count", "", 1, 1, tagset.CompositeTags{}, 2, metrics.CountType).Ref
	s.SetSeriesKind(second, stable)
	generation = s.SeriesGeneration()
	writes = s.WriteGeneration(ref)
	s.ResetSeriesKinds()
	require.Equal(t, generation+1, s.SeriesGeneration(), "reset invalidates once, not per changed kind")
	require.Equal(t, writes, s.WriteGeneration(ref), "kind reset must not count as a point write")
	require.Equal(t, history, s.GetSeriesRange(ref, 0, 3, observer.AggregateAverage))
	generation = s.SeriesGeneration()
	s.ResetSeriesKinds()
	require.Equal(t, generation, s.SeriesGeneration(), "no-op reset must not invalidate")
	require.Empty(t, s.ListSeriesRefsInto(filter, nil))
	require.Equal(t, 3, s.PointCountUpTo(ref, 3))
	unset := observer.UNSET
	filter.MetricKind = &unset
	require.Len(t, s.ListSeries(filter), 1)
	generation = s.SeriesGeneration()
	s.RemoveSeriesByRefs([]observer.SeriesRef{ref})
	require.Equal(t, generation+1, s.SeriesGeneration())
	generation = s.SeriesGeneration()
	s.RemoveSeriesByRefs([]observer.SeriesRef{ref, 999})
	require.Equal(t, generation, s.SeriesGeneration())
	_, ok = s.SeriesKind(ref)
	require.False(t, ok)
	require.Empty(t, s.ListSeries(filter))
}

func TestStorageMetricKindFilters(t *testing.T) {
	s := newTimeSeriesStorage()
	add := func(key uint64, namespace, name, host string, mt metrics.MetricType, kind observer.MetricKind) observer.SeriesRef {
		ref := s.AddWithKeyAndHostComposite(namespace, name, host, 10, 1,
			tagset.NewCompositeTags([]string{"env:test"}, nil), key, mt).Ref
		s.SetSeriesKind(ref, kind)
		return ref
	}
	stableGauge := add(1, "check", "cpu.user", "host-a", metrics.GaugeType, observer.STABLE)
	linearGauge := add(2, "check", "cpu.system", "host-b", metrics.GaugeType, observer.LINEAR)
	stableCount := add(3, "check", "requests", "host-a", metrics.CountType, observer.STABLE)
	unknown := add(4, "check", "untyped", "host-a", observer.UnknownType, observer.UNSET)
	telemetry := add(5, observer.TelemetryNamespace, "internal", "host-a", metrics.GaugeType, observer.STABLE)
	stable, gauge := observer.STABLE, metrics.GaugeType
	unset, unknownType := observer.UNSET, observer.UnknownType

	for _, tc := range []struct {
		name   string
		filter observer.SeriesFilter
		want   []observer.SeriesRef
	}{
		{"unrestricted", observer.SeriesFilter{}, []observer.SeriesRef{stableGauge, linearGauge, stableCount, unknown, telemetry}},
		{"kind only", observer.SeriesFilter{MetricKind: &stable}, []observer.SeriesRef{stableGauge, stableCount, telemetry}},
		{"type only", observer.SeriesFilter{MetricType: &gauge}, []observer.SeriesRef{stableGauge, linearGauge, telemetry}},
		{"unset", observer.SeriesFilter{MetricKind: &unset}, []observer.SeriesRef{unknown}},
		{"unknown input type", observer.SeriesFilter{MetricType: &unknownType}, []observer.SeriesRef{unknown}},
		{"workload intersection", observer.SeriesFilter{
			MetricKind: &stable, MetricType: &gauge, ExcludeNamespaces: []string{observer.TelemetryNamespace},
		}, []observer.SeriesRef{stableGauge}},
		{"namespace overrides exclusion", observer.SeriesFilter{
			MetricKind: &stable, Namespace: observer.TelemetryNamespace, ExcludeNamespaces: []string{observer.TelemetryNamespace},
		}, []observer.SeriesRef{telemetry}},
		{"existing criteria", observer.SeriesFilter{
			MetricType: &gauge, Namespace: "check", NamePattern: "cpu.", Host: "host-b", TagMatchers: map[string]string{"env": "test"},
		}, []observer.SeriesRef{linearGauge}},
		{"tag mismatch", observer.SeriesFilter{
			MetricKind: &stable, TagMatchers: map[string]string{"env": "prod"},
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var refs []observer.SeriesRef
			for _, meta := range s.ListSeries(tc.filter) {
				refs = append(refs, meta.Ref)
			}
			require.ElementsMatch(t, tc.want, refs)
			require.ElementsMatch(t, tc.want, s.ListSeriesRefsInto(tc.filter, nil))
		})
	}

	// Promotion changes eligibility without admitting another series or changing its kind.
	generation := s.SeriesGeneration()
	result := s.AddWithKeyAndHostComposite("check", "untyped", "host-a", 11, 2, tagset.CompositeTags{}, 4, metrics.GaugeType)
	require.Equal(t, unknown, result.Ref)
	require.False(t, result.IsNew)
	require.Equal(t, generation+1, s.SeriesGeneration())
	require.ElementsMatch(t, []observer.SeriesRef{stableGauge, linearGauge, unknown, telemetry},
		s.ListSeriesRefsInto(observer.SeriesFilter{MetricType: &gauge}, nil))
	kind, ok := s.SeriesKind(unknown)
	require.True(t, ok)
	require.Equal(t, observer.UNSET, kind)
}
