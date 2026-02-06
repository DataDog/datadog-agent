// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	series map[string]*seriesStats

	// Drop accounting for invalid/unsafe input values.
	droppedNonFinite int64
	droppedExtreme   int64
	droppedByMetric  map[string]int64
	sampledDrops     map[string]int
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

// Aggregate is an alias to the definition in the observer component for internal use.
type Aggregate = observer.Aggregate

// Re-export aggregate constants for internal use.
const (
	AggregateAverage = observer.AggregateAverage
	AggregateSum     = observer.AggregateSum
	AggregateCount   = observer.AggregateCount
	AggregateMin     = observer.AggregateMin
	AggregateMax     = observer.AggregateMax
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
	points := make([]observer.Point, 0, len(s.Points))
	for _, p := range s.Points {
		points = append(points, observer.Point{
			Timestamp: p.Timestamp,
			Value:     p.aggregate(agg),
		})
	}
	return observer.Series{
		Namespace: s.Namespace,
		Name:      s.Name,
		Tags:      s.Tags,
		Points:    points,
	}
}

// toSeriesUpTo returns a Series with only points where Timestamp <= upTo.
func (s *seriesStats) toSeriesUpTo(agg Aggregate, upTo int64) observer.Series {
	points := make([]observer.Point, 0, len(s.Points))
	for _, p := range s.Points {
		if p.Timestamp <= upTo {
			points = append(points, observer.Point{
				Timestamp: p.Timestamp,
				Value:     p.aggregate(agg),
			})
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
		series:          make(map[string]*seriesStats),
		droppedByMetric: make(map[string]int64),
		sampledDrops:    make(map[string]int),
	}
}

// Add records a data point for a named metric in a namespace.
// Invalid values are dropped at ingest with accounting and sampled logging.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		s.recordDroppedValue("non_finite", namespace, name, value, timestamp, tags)
		return
	}
	// Guard against known finite sentinel values (MaxFloat64 used as "unlimited")
	// that overflow downstream aggregation math when summed.
	if value == math.MaxFloat64 {
		s.recordDroppedValue("extreme", namespace, name, value, timestamp, tags)
		return
	}
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

func (s *timeSeriesStorage) recordDroppedValue(reason, namespace, name string, value float64, timestamp int64, tags []string) {
	switch reason {
	case "non_finite":
		s.droppedNonFinite++
	case "extreme":
		s.droppedExtreme++
	}

	metricKey := namespace + "|" + name
	s.droppedByMetric[metricKey]++
	sampled := s.sampledDrops[metricKey]
	if sampled < 3 {
		s.sampledDrops[metricKey] = sampled + 1
		log.Printf("[observer] dropped %s metric value namespace=%q metric=%q value=%g ts=%d tags=%v sample=%d",
			reason, namespace, name, value, timestamp, tags, sampled+1)
	}
}

func (s *timeSeriesStorage) DroppedValueStats() (nonFinite int64, extreme int64, byMetric map[string]int64) {
	byMetric = make(map[string]int64, len(s.droppedByMetric))
	for k, v := range s.droppedByMetric {
		byMetric[k] = v
	}
	return s.droppedNonFinite, s.droppedExtreme, byMetric
}

// GetSeries returns the series using the specified aggregation.
// If tags is nil, finds the first series matching namespace and name (ignoring tags).
func (s *timeSeriesStorage) GetSeries(namespace, name string, tags []string, agg Aggregate) *observer.Series {
	if tags != nil {
		// Exact match with tags
		key := seriesKey(namespace, name, tags)
		stats := s.series[key]
		if stats == nil {
			return nil
		}
		series := stats.toSeries(agg)
		return &series
	}

	// tags is nil: find first matching series by namespace and name
	prefix := namespace + "|" + name + "|"
	for key, stats := range s.series {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			series := stats.toSeries(agg)
			return &series
		}
	}
	return nil
}

// GetSeriesSince returns points with timestamp > since (for delta updates).
// If since is 0, returns all points.
func (s *timeSeriesStorage) GetSeriesSince(namespace, name string, tags []string, agg Aggregate, since int64) *observer.Series {
	key := seriesKey(namespace, name, tags)
	stats := s.series[key]
	if stats == nil {
		return nil
	}

	// If since is 0, return all points
	if since == 0 {
		series := stats.toSeries(agg)
		return &series
	}

	// Filter points to only those after 'since'
	var points []observer.Point
	for _, p := range stats.Points {
		if p.Timestamp > since {
			points = append(points, observer.Point{
				Timestamp: p.Timestamp,
				Value:     p.aggregate(agg),
			})
		}
	}

	return &observer.Series{
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
		Points:    points,
	}
}

// Namespaces returns the set of namespaces that have data.
func (s *timeSeriesStorage) Namespaces() []string {
	seen := make(map[string]struct{})
	for _, stats := range s.series {
		seen[stats.Namespace] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for ns := range seen {
		result = append(result, ns)
	}
	sort.Strings(result)
	return result
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

// TimeBounds returns the minimum and maximum timestamps across all stored points.
func (s *timeSeriesStorage) TimeBounds() (minTs int64, maxTs int64, ok bool) {
	var min int64
	var max int64
	found := false

	for _, stats := range s.series {
		for _, p := range stats.Points {
			if !found {
				min = p.Timestamp
				max = p.Timestamp
				found = true
				continue
			}
			if p.Timestamp < min {
				min = p.Timestamp
			}
			if p.Timestamp > max {
				max = p.Timestamp
			}
		}
	}

	return min, max, found
}

// MaxTimestamp returns the latest timestamp across all series in storage.
func (s *timeSeriesStorage) MaxTimestamp() int64 {
	var max int64
	for _, stats := range s.series {
		if n := len(stats.Points); n > 0 {
			if t := stats.Points[n-1].Timestamp; t > max {
				max = t
			}
		}
	}
	return max
}

// seriesKey creates a unique key for a series.
func seriesKey(namespace, name string, tags []string) string {
	sortedTags := copyTags(tags)
	sort.Strings(sortedTags)
	return namespace + "|" + name + "|" + strings.Join(sortedTags, ",")
}

// parseSeriesKey parses a series key back into its parts.
func parseSeriesKey(key string) (namespace, name string, tags []string, ok bool) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return "", "", nil, false
	}
	namespace = parts[0]
	name = parts[1]
	if parts[2] == "" {
		return namespace, name, nil, true
	}
	tags = strings.Split(parts[2], ",")
	return namespace, name, tags, true
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

// ListAllSeriesCompact returns lightweight metadata for every stored series.
func (s *timeSeriesStorage) ListAllSeriesCompact() []seriesCompact {
	result := make([]seriesCompact, 0, len(s.series))
	for _, st := range s.series {
		result = append(result, seriesCompact{
			Namespace: st.Namespace,
			Name:      st.Name,
			Tags:      st.Tags,
		})
	}
	return result
}

// DumpToFile writes all series to a JSON file for debugging.
func (s *timeSeriesStorage) DumpToFile(path string) error {
	type dumpPoint struct {
		Timestamp int64   `json:"ts"`
		Sum       float64 `json:"sum"`
		Count     int64   `json:"count"`
		Min       float64 `json:"min"`
		Max       float64 `json:"max"`
	}
	type dumpSeries struct {
		Namespace string      `json:"namespace"`
		Name      string      `json:"name"`
		Tags      []string    `json:"tags"`
		Points    []dumpPoint `json:"points"`
	}

	var out []dumpSeries
	for _, st := range s.series {
		ds := dumpSeries{
			Namespace: st.Namespace,
			Name:      st.Name,
			Tags:      st.Tags,
		}
		for _, p := range st.Points {
			ds.Points = append(ds.Points, dumpPoint{
				Timestamp: p.Timestamp,
				Sum:       p.Sum,
				Count:     p.Count,
				Min:       p.Min,
				Max:       p.Max,
			})
		}
		out = append(out, ds)
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// StorageReader interface implementation

// ListSeries returns keys of all series matching the filter.
func (s *timeSeriesStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesKey {
	var result []observer.SeriesKey
	for _, stats := range s.series {
		// Check namespace filter
		if filter.Namespace != "" && stats.Namespace != filter.Namespace {
			continue
		}
		// Check name pattern filter (prefix match)
		if filter.NamePattern != "" && !strings.HasPrefix(stats.Name, filter.NamePattern) {
			continue
		}
		// Check tag matchers
		if !matchTags(stats.Tags, filter.TagMatchers) {
			continue
		}
		result = append(result, observer.SeriesKey{
			Namespace: stats.Namespace,
			Name:      stats.Name,
			Tags:      stats.Tags,
		})
	}
	return result
}

// PointCount returns the number of raw data points for a series.
func (s *timeSeriesStorage) PointCount(key observer.SeriesKey) int {
	k := seriesKey(key.Namespace, key.Name, key.Tags)
	if stats, ok := s.series[k]; ok {
		return len(stats.Points)
	}
	return 0
}

// matchTags checks if tags contain all required key=value pairs.
func matchTags(tags []string, matchers map[string]string) bool {
	if len(matchers) == 0 {
		return true
	}
	tagMap := make(map[string]string)
	for _, t := range tags {
		if idx := strings.Index(t, ":"); idx > 0 {
			tagMap[t[:idx]] = t[idx+1:]
		}
	}
	for k, v := range matchers {
		if tagMap[k] != v {
			return false
		}
	}
	return true
}

// GetSeriesByKey returns the full series for a key.
func (s *timeSeriesStorage) GetSeriesByKey(key observer.SeriesKey, agg Aggregate) *observer.Series {
	internalKey := seriesKey(key.Namespace, key.Name, key.Tags)
	stats := s.series[internalKey]
	if stats == nil {
		return nil
	}
	series := stats.toSeries(agg)
	return &series
}

// GetSeriesRange returns points within a time range [start, end].
func (s *timeSeriesStorage) GetSeriesRange(key observer.SeriesKey, start, end int64, agg Aggregate) *observer.Series {
	internalKey := seriesKey(key.Namespace, key.Name, key.Tags)
	stats := s.series[internalKey]
	if stats == nil {
		return nil
	}

	points := make([]observer.Point, 0)
	for _, p := range stats.Points {
		if p.Timestamp >= start && p.Timestamp <= end {
			points = append(points, observer.Point{
				Timestamp: p.Timestamp,
				Value:     p.aggregate(agg),
			})
		}
	}
	return &observer.Series{
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
		Points:    points,
	}
}

// ReadSince returns points with timestamp > cursor, plus the new cursor position.
func (s *timeSeriesStorage) ReadSince(key observer.SeriesKey, cursor int64, agg Aggregate) ([]observer.Point, int64) {
	internalKey := seriesKey(key.Namespace, key.Name, key.Tags)
	stats := s.series[internalKey]
	if stats == nil {
		return nil, cursor
	}

	var points []observer.Point
	newCursor := cursor
	for _, p := range stats.Points {
		if p.Timestamp > cursor {
			points = append(points, observer.Point{
				Timestamp: p.Timestamp,
				Value:     p.aggregate(agg),
			})
			if p.Timestamp > newCursor {
				newCursor = p.Timestamp
			}
		}
	}
	return points, newCursor
}
