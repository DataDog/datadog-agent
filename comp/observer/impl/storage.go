// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	series map[string]*seriesStats
}

// seriesStats contains accumulated statistics for a time series (internal).
type seriesStats struct {
	Namespace string
	Name      string
	Tags      []string
	Points    []statPoint
}

// statPoint holds summary statistics for a single time bucket (internal).
type statPoint struct {
	Timestamp int64
	Sum       float64
	Count     int64
	Min       float64
	Max       float64
}

// Aggregate specifies which statistic to extract from summary stats.
type Aggregate int

const (
	AggregateAverage Aggregate = iota
	AggregateSum
	AggregateCount
	AggregateMin
	AggregateMax
)

// aggregate extracts the specified statistic from this point.
func (p *statPoint) aggregate(agg Aggregate) float64 {
	switch agg {
	case AggregateAverage:
		if p.Count == 0 {
			return 0
		}
		return p.Sum / float64(p.Count)
	case AggregateSum:
		return p.Sum
	case AggregateCount:
		return float64(p.Count)
	case AggregateMin:
		return p.Min
	case AggregateMax:
		return p.Max
	default:
		return 0
	}
}

// toSeries converts internal stats to the simplified Series for analyses.
func (s *seriesStats) toSeries(agg Aggregate) observer.Series {
	points := make([]observer.Point, len(s.Points))
	for i, p := range s.Points {
		points[i] = observer.Point{
			Timestamp: p.Timestamp,
			Value:     p.aggregate(agg),
		}
	}
	return observer.Series{
		Namespace: s.Namespace,
		Name:      s.Name,
		Tags:      s.Tags,
		Points:    points,
	}
}

// newTimeSeriesStorage creates a new time series storage.
func newTimeSeriesStorage() *timeSeriesStorage {
	return &timeSeriesStorage{
		series: make(map[string]*seriesStats),
	}
}

// Add records a data point for a named metric in a namespace.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) {
	key := seriesKey(namespace, name, tags)

	stats, exists := s.series[key]
	if !exists {
		stats = &seriesStats{
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
		stats.Points = append(stats.Points, statPoint{
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
}

// GetSeries returns the series using the specified aggregation.
func (s *timeSeriesStorage) GetSeries(namespace, name string, tags []string, agg Aggregate) *observer.Series {
	key := seriesKey(namespace, name, tags)
	stats := s.series[key]
	if stats == nil {
		return nil
	}
	series := stats.toSeries(agg)
	return &series
}

// AllSeries returns all series in a namespace using the specified aggregation.
func (s *timeSeriesStorage) AllSeries(namespace string, agg Aggregate) []observer.Series {
	var result []observer.Series
	for _, stats := range s.series {
		if stats.Namespace == namespace {
			result = append(result, stats.toSeries(agg))
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
