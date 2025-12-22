// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	series map[string]*observer.SeriesStats
}

// newTimeSeriesStorage creates a new time series storage.
func newTimeSeriesStorage() *timeSeriesStorage {
	return &timeSeriesStorage{
		series: make(map[string]*observer.SeriesStats),
	}
}

// Add records a data point for a named metric in a namespace.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) *observer.SeriesStats {
	key := seriesKey(namespace, name, tags)

	stats, exists := s.series[key]
	if !exists {
		stats = &observer.SeriesStats{
			Namespace: namespace,
			Name:      name,
			Tags:      copyTags(tags),
			Points:    nil,
		}
		s.series[key] = stats
	}

	// Bucket by second
	bucket := timestamp

	// Find or create the bucket
	idx := -1
	for i, p := range stats.Points {
		if p.Timestamp == bucket {
			idx = i
			break
		}
	}

	if idx >= 0 {
		// Update existing bucket
		stats.Points[idx].Sum += value
		stats.Points[idx].Count++
		if value < stats.Points[idx].Min {
			stats.Points[idx].Min = value
		}
		if value > stats.Points[idx].Max {
			stats.Points[idx].Max = value
		}
	} else {
		// Create new bucket
		stats.Points = append(stats.Points, observer.StatPoint{
			Timestamp: bucket,
			Sum:       value,
			Count:     1,
			Min:       value,
			Max:       value,
		})
		// Keep points sorted by timestamp
		sort.Slice(stats.Points, func(i, j int) bool {
			return stats.Points[i].Timestamp < stats.Points[j].Timestamp
		})
	}

	return stats
}

// GetSeries returns the accumulated stats for a series.
func (s *timeSeriesStorage) GetSeries(namespace, name string, tags []string) *observer.SeriesStats {
	key := seriesKey(namespace, name, tags)
	return s.series[key]
}

// AllSeries returns all series in a namespace.
func (s *timeSeriesStorage) AllSeries(namespace string) []*observer.SeriesStats {
	var result []*observer.SeriesStats
	for _, stats := range s.series {
		if stats.Namespace == namespace {
			result = append(result, stats)
		}
	}
	return result
}

// seriesKey creates a unique key for a series.
func seriesKey(namespace, name string, tags []string) string {
	sortedTags := copyTags(tags)
	sort.Strings(sortedTags)
	return namespace + "|" + name + "|" + strings.Join(sortedTags, ",")
}

// copyTags creates a copy of tags slice.
func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	result := make([]string, len(tags))
	copy(result, tags)
	return result
}

// floatMin returns the minimum of two floats.
func floatMin(a, b float64) float64 {
	return math.Min(a, b)
}

// floatMax returns the maximum of two floats.
func floatMax(a, b float64) float64 {
	return math.Max(a, b)
}
