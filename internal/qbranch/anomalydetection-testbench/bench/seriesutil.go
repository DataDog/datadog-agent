// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"sort"
	"strconv"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

// aggSuffix returns the short string representation of an aggregate.
func aggSuffix(agg observerdef.Aggregate) string {
	return observerdef.AggregateString(agg)
}

// seriesKey returns a canonical string key for a series:
// "namespace|name:agg|tag1,tag2,..."
func seriesKey(namespace, nameWithAgg string, tags []string) string {
	if len(tags) == 0 {
		return namespace + "|" + nameWithAgg + "|"
	}
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return namespace + "|" + nameWithAgg + "|" + strings.Join(sorted, ",")
}

// parseSeriesKey parses a seriesKey back into its components.
// Returns ok=false if the key doesn't have the expected format.
func parseSeriesKey(key string) (namespace, name string, tags []string, ok bool) {
	// Format: "namespace|name:agg|tags"
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return "", "", nil, false
	}
	namespace = parts[0]
	name = parts[1]
	if parts[2] != "" {
		tags = strings.Split(parts[2], ",")
	}
	return namespace, name, tags, true
}

// stateViewStorage adapts a StateView to provide compact series ID lookups and
// series-by-numeric-ID retrieval. This replaces the private timeSeriesStorage
// methods used in the original testbench API.
type stateViewStorage struct {
	sv observerimpl.StateView
}

// listNamespaces returns all distinct namespaces present in the state view.
func (s *stateViewStorage) listNamespaces() []string {
	series := s.sv.ListSeries(observerdef.SeriesFilter{})
	seen := make(map[string]struct{})
	var result []string
	for _, m := range series {
		if _, ok := seen[m.Namespace]; !ok {
			seen[m.Namespace] = struct{}{}
			result = append(result, m.Namespace)
		}
	}
	sort.Strings(result)
	return result
}

// listSeriesForNamespace returns series metadata for a given namespace.
func (s *stateViewStorage) listSeriesForNamespace(ns string) []observerdef.SeriesMeta {
	return s.sv.ListSeries(observerdef.SeriesFilter{Namespace: ns})
}

// getSeriesByNumericID finds a series by its numeric ref and returns data with the given agg.
func (s *stateViewStorage) getSeriesByNumericID(ref observerdef.SeriesRef, agg observerdef.Aggregate) *observerdef.Series {
	maxTs := s.sv.MaxTimestamp()
	return s.sv.GetSeriesRange(ref, 0, maxTs, agg)
}

// getSeriesMeta returns the SeriesMeta for a given ref, or nil if not found.
func (s *stateViewStorage) getSeriesMeta(ref observerdef.SeriesRef) *observerdef.SeriesMeta {
	all := s.sv.ListSeries(observerdef.SeriesFilter{})
	for i := range all {
		if all[i].Ref == ref {
			return &all[i]
		}
	}
	return nil
}

// compactSeriesID maps a full seriesKey to a compact numeric ID ("42:avg").
// Returns the original key if not found (to match the original behavior).
func (s *stateViewStorage) compactSeriesID(fullKey string) string {
	namespace, nameWithAgg, tags, ok := parseSeriesKey(fullKey)
	if !ok {
		return fullKey
	}

	// Separate name from agg suffix.
	name := nameWithAgg
	aggStr := "avg"
	if idx := strings.LastIndex(nameWithAgg, ":"); idx > 0 {
		name = nameWithAgg[:idx]
		aggStr = nameWithAgg[idx+1:]
	}

	filter := observerdef.SeriesFilter{Namespace: namespace}
	series := s.sv.ListSeries(filter)

	// Sort tags for comparison.
	sortedTags := make([]string, len(tags))
	copy(sortedTags, tags)
	sort.Strings(sortedTags)

	for _, m := range series {
		if m.Name != name {
			continue
		}
		// Compare tags.
		mTags := make([]string, len(m.Tags))
		copy(mTags, m.Tags)
		sort.Strings(mTags)
		if tagsMatch(mTags, sortedTags) {
			return strconv.Itoa(int(m.Ref)) + ":" + aggStr
		}
	}

	return fullKey
}

func tagsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
