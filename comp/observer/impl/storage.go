// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	mu     sync.RWMutex
	series map[string]*seriesStats

	// observationTimestamps tracks all timestamps where observations occurred,
	// even if no metric series was written for that timestamp.
	observationTimestamps map[int64]struct{}

	// Compact numeric IDs for API responses; unrelated to generation tracking.
	seriesIDs    map[string]int // internal key → numeric ID
	seriesIDKeys []string       // numeric ID → internal key (index = ID)

	// Global generation for the series catalog; increments only when a new
	// series key is created, not on every write to an existing series.
	seriesGen uint64

	// Drop accounting for invalid/unsafe input values.
	droppedNonFinite int64
	droppedExtreme   int64
	droppedByMetric  map[string]int64
	sampledDrops     map[string]int
}

// seriesStats contains accumulated statistics for a time series (internal).
// Data is stored in columnar layout: parallel arrays indexed by point position.
// Timestamps are stored in sorted order, enabling binary search for range queries.
type seriesStats struct {
	Namespace string
	Name      string
	Tags      []string

	// writeGeneration is per-series and increments on every Add, including
	// same-bucket merges into an existing point.
	writeGeneration int64

	// Columnar storage — all slices have the same length, indexed by point position.
	timestamps []int64
	sums       []float64
	counts     []int64
	mins       []float64
	maxes      []float64
}

// pointCount returns the number of stored points.
func (s *seriesStats) pointCount() int {
	return len(s.timestamps)
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

// aggregateColumn returns the pre-materialized column values for a given aggregate.
// For Average, it computes sum/count on the fly. For others, it returns the column directly.
func (s *seriesStats) aggregateColumn(agg Aggregate) []float64 {
	switch agg {
	case AggregateSum:
		return s.sums
	case AggregateMin:
		return s.mins
	case AggregateMax:
		return s.maxes
	case AggregateCount:
		vals := make([]float64, len(s.counts))
		for i, c := range s.counts {
			vals[i] = float64(c)
		}
		return vals
	case AggregateAverage:
		vals := make([]float64, len(s.sums))
		for i := range s.sums {
			if s.counts[i] == 0 {
				vals[i] = 0
			} else {
				vals[i] = s.sums[i] / float64(s.counts[i])
			}
		}
		return vals
	default:
		return make([]float64, len(s.timestamps))
	}
}

// aggregateAt extracts the specified statistic at index i.
func (s *seriesStats) aggregateAt(i int, agg Aggregate) float64 {
	switch agg {
	case AggregateAverage:
		if s.counts[i] == 0 {
			return 0
		}
		return s.sums[i] / float64(s.counts[i])
	case AggregateSum:
		return s.sums[i]
	case AggregateCount:
		return float64(s.counts[i])
	case AggregateMin:
		return s.mins[i]
	case AggregateMax:
		return s.maxes[i]
	default:
		return 0
	}
}

// toSeries converts internal stats to the simplified Series for analyses.
func (s *seriesStats) toSeries(agg Aggregate) observer.Series {
	n := s.pointCount()
	points := make([]observer.Point, n)
	col := s.aggregateColumn(agg)
	for i := 0; i < n; i++ {
		points[i] = observer.Point{
			Timestamp: s.timestamps[i],
			Value:     col[i],
		}
	}
	return observer.Series{
		Namespace: s.Namespace,
		Name:      s.Name,
		Tags:      s.Tags,
		Points:    points,
	}
}

// toSeriesUpTo returns a Series with only points where Timestamp <= upTo.
// Uses binary search on the sorted timestamps column.
func (s *seriesStats) toSeriesUpTo(agg Aggregate, upTo int64) observer.Series {
	end := sort.Search(len(s.timestamps), func(i int) bool {
		return s.timestamps[i] > upTo
	})
	points := make([]observer.Point, end)
	col := s.aggregateColumn(agg)
	for i := 0; i < end; i++ {
		points[i] = observer.Point{
			Timestamp: s.timestamps[i],
			Value:     col[i],
		}
	}
	return observer.Series{
		Namespace: s.Namespace,
		Name:      s.Name,
		Tags:      s.Tags,
		Points:    points,
	}
}

// searchAfter returns the index of the first timestamp > value using binary search.
func searchAfter(timestamps []int64, value int64) int {
	return sort.Search(len(timestamps), func(i int) bool {
		return timestamps[i] > value
	})
}

// searchAtOrAfter returns the index of the first timestamp >= value using binary search.
func searchAtOrAfter(timestamps []int64, value int64) int {
	return sort.Search(len(timestamps), func(i int) bool {
		return timestamps[i] >= value
	})
}

// newTimeSeriesStorage creates a new time series storage.
func newTimeSeriesStorage() *timeSeriesStorage {
	return &timeSeriesStorage{
		series:                make(map[string]*seriesStats),
		observationTimestamps: make(map[int64]struct{}),
		seriesIDs:             make(map[string]int),
		droppedByMetric:       make(map[string]int64),
		sampledDrops:          make(map[string]int),
	}
}

// Add records a data point for a named metric in a namespace.
// Invalid values are dropped at ingest with accounting and sampled logging.
// Timestamps are maintained in sorted order so replay and live ingestion remain
// correct even when data arrives out of order.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if math.IsInf(value, 0) || math.IsNaN(value) {
		s.recordDroppedValue("non_finite", namespace, name, value, timestamp, tags)
		return
	}
	// Guard against known finite sentinel values (MaxFloat64 used as "unlimited")
	// that overflow downstream aggregation math when summed.
	if value == math.MaxFloat64 || value == -math.MaxFloat64 {
		s.recordDroppedValue("extreme", namespace, name, value, timestamp, tags)
		return
	}
	key := seriesKey(namespace, name, tags)

	stats, exists := s.series[key]
	if !exists {
		stats = &seriesStats{
			Namespace: namespace,
			Name:      name,
			Tags:      canonicalizeTags(tags),
		}
		s.series[key] = stats
		// Assign a compact numeric ID for API responses.
		id := len(s.seriesIDKeys)
		s.seriesIDs[key] = id
		s.seriesIDKeys = append(s.seriesIDKeys, key)
		s.seriesGen++
	}
	stats.writeGeneration++

	// Bucket by second.
	bucket := timestamp

	// Binary search for the bucket in the sorted timestamps array.
	idx := sort.Search(len(stats.timestamps), func(i int) bool {
		return stats.timestamps[i] >= bucket
	})

	if idx < len(stats.timestamps) && stats.timestamps[idx] == bucket {
		// Update existing bucket in-place.
		stats.sums[idx] += value
		stats.counts[idx]++
		if value < stats.mins[idx] {
			stats.mins[idx] = value
		}
		if value > stats.maxes[idx] {
			stats.maxes[idx] = value
		}
		return
	}

	stats.timestamps = insertInt64(stats.timestamps, idx, bucket)
	stats.sums = insertFloat64(stats.sums, idx, value)
	stats.counts = insertInt64(stats.counts, idx, 1)
	stats.mins = insertFloat64(stats.mins, idx, value)
	stats.maxes = insertFloat64(stats.maxes, idx, value)
}

// insertInt64 inserts v at position idx in s, maintaining order.
func insertInt64(s []int64, idx int, v int64) []int64 {
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
}

// insertFloat64 inserts v at position idx in s, maintaining order.
func insertFloat64(s []float64, idx int, v float64) []float64 {
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	byMetric = make(map[string]int64, len(s.droppedByMetric))
	for k, v := range s.droppedByMetric {
		byMetric[k] = v
	}
	return s.droppedNonFinite, s.droppedExtreme, byMetric
}

// GetSeries returns the series using the specified aggregation.
// If tags is nil, finds the first series matching namespace and name (ignoring tags).
func (s *timeSeriesStorage) GetSeries(namespace, name string, tags []string, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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

	// Binary search for the first timestamp > since.
	startIdx := searchAfter(stats.timestamps, since)

	n := stats.pointCount()
	points := make([]observer.Point, 0, n-startIdx)
	for i := startIdx; i < n; i++ {
		points = append(points, observer.Point{
			Timestamp: stats.timestamps[i],
			Value:     stats.aggregateAt(i, agg),
		})
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

	var min int64
	var max int64
	found := false

	for _, stats := range s.series {
		n := stats.pointCount()
		if n == 0 {
			continue
		}
		// Timestamps are sorted, but some series may start with default/non-data
		// zero timestamps. Ignore only the non-positive prefix, not the series.
		firstIdx := searchAfter(stats.timestamps, 0)
		if firstIdx >= n {
			continue
		}
		first := stats.timestamps[firstIdx]
		last := stats.timestamps[n-1]
		if !found {
			min = first
			max = last
			found = true
		} else {
			if first < min {
				min = first
			}
			if last > max {
				max = last
			}
		}
	}

	return min, max, found
}

// MaxTimestamp returns the latest timestamp across all series in storage.
func (s *timeSeriesStorage) MaxTimestamp() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var max int64
	for _, stats := range s.series {
		if n := stats.pointCount(); n > 0 {
			if t := stats.timestamps[n-1]; t > max {
				max = t
			}
		}
	}
	return max
}

// seriesKey creates a unique key for a series.
func seriesKey(namespace, name string, tags []string) string {
	if len(tags) > 1 && !tagsSorted(tags) {
		tags = canonicalizeTags(tags)
	}
	return namespace + "|" + name + "|" + joinTags(tags)
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

func canonicalizeTags(tags []string) []string {
	if len(tags) <= 1 {
		return copyTags(tags)
	}
	result := copyTags(tags)
	sort.Strings(result)
	return result
}

func tagsSorted(tags []string) bool {
	for i := 1; i < len(tags); i++ {
		if tags[i-1] > tags[i] {
			return false
		}
	}
	return true
}

func joinTags(tags []string) string {
	switch len(tags) {
	case 0:
		return ""
	case 1:
		return tags[0]
	default:
		return strings.Join(tags, ",")
	}
}

// seriesMeta is lightweight series metadata including point count,
// used for API listing without materializing point data.
type seriesMeta struct {
	ID         int // compact numeric ID
	Namespace  string
	Name       string
	Tags       []string
	PointCount int
}

// ListSeriesMetadata returns lightweight metadata for all series in a namespace.
// Unlike AllSeries, this does not materialize point data — it only reads point counts.
func (s *timeSeriesStorage) ListSeriesMetadata(namespace string) []seriesMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []seriesMeta
	for key, stats := range s.series {
		if stats.Namespace == namespace {
			result = append(result, seriesMeta{
				ID:         s.seriesIDs[key],
				Namespace:  stats.Namespace,
				Name:       stats.Name,
				Tags:       copyTags(stats.Tags),
				PointCount: stats.pointCount(),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return strings.Join(result[i].Tags, ",") < strings.Join(result[j].Tags, ",")
	})
	return result
}

// GetSeriesByNumericID looks up a series by its compact numeric ID and returns
// the data using the specified aggregation.
func (s *timeSeriesStorage) GetSeriesByNumericID(id int, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if id < 0 || id >= len(s.seriesIDKeys) {
		return nil
	}
	key := s.seriesIDKeys[id]
	stats := s.series[key]
	if stats == nil {
		return nil
	}
	series := stats.toSeries(agg)
	return &series
}

// ListAllSeriesCompact returns lightweight metadata for every stored series.
func (s *timeSeriesStorage) ListAllSeriesCompact() []seriesCompact {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

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
		n := st.pointCount()
		for i := 0; i < n; i++ {
			ds.Points = append(ds.Points, dumpPoint{
				Timestamp: st.timestamps[i],
				Sum:       st.sums[i],
				Count:     st.counts[i],
				Min:       st.mins[i],
				Max:       st.maxes[i],
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

// DataTimestamps returns all unique timestamps that have data, sorted ascending.
func (s *timeSeriesStorage) DataTimestamps() []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := make(map[int64]struct{})
	for _, stats := range s.series {
		for _, ts := range stats.timestamps {
			seen[ts] = struct{}{}
		}
	}
	// Include observation timestamps (e.g., from logs that produced no virtual metrics).
	for ts := range s.observationTimestamps {
		seen[ts] = struct{}{}
	}

	timestamps := make([]int64, 0, len(seen))
	for ts := range seen {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	return timestamps
}

// SeriesGeneration returns a counter that increments whenever a new series key
// is created. Callers can use this to safely cache ListSeries results.
func (s *timeSeriesStorage) SeriesGeneration() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seriesGen
}

// CompactSeriesID translates a full series key (as used in SourceSeriesID) to its
// compact numeric ID string. The full key format is "namespace|name:agg|tags" where
// the storage key is "namespace|name|tags" (without the agg suffix). This method
// strips the agg suffix, looks up the numeric ID, and returns "numericID:agg".
// Returns the original key unchanged if no mapping exists.
func (s *timeSeriesStorage) CompactSeriesID(fullKey string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	namespace, nameWithAgg, tags, ok := parseSeriesKey(fullKey)
	if !ok {
		return fullKey
	}

	// Split off the aggregation suffix from the name.
	baseName := nameWithAgg
	aggStr := ""
	if idx := strings.LastIndex(nameWithAgg, ":"); idx > 0 {
		baseName = nameWithAgg[:idx]
		aggStr = nameWithAgg[idx+1:]
	}

	// Look up the storage key (without agg suffix).
	storageKey := seriesKey(namespace, baseName, tags)
	numID, found := s.seriesIDs[storageKey]
	if !found {
		return fullKey
	}

	if aggStr != "" {
		return fmt.Sprintf("%d:%s", numID, aggStr)
	}
	return fmt.Sprintf("%d", numID)
}

// FullKeyForNumericID returns the internal storage key for a compact numeric ID.
func (s *timeSeriesStorage) FullKeyForNumericID(id int) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if id < 0 || id >= len(s.seriesIDKeys) {
		return "", false
	}
	return s.seriesIDKeys[id], true
}

// StorageReader interface implementation

// ListSeries returns keys of all series matching the filter.
func (s *timeSeriesStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesKey {
	s.mu.RLock()
	defer s.mu.RUnlock()

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
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := seriesKey(key.Namespace, key.Name, key.Tags)
	if stats, ok := s.series[k]; ok {
		return stats.pointCount()
	}
	return 0
}

// PointCountUpTo returns the number of raw data points with timestamp <= endTime.
// Uses binary search since timestamps are sorted.
func (s *timeSeriesStorage) PointCountUpTo(key observer.SeriesKey, endTime int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := seriesKey(key.Namespace, key.Name, key.Tags)
	stats, ok := s.series[k]
	if !ok || stats.pointCount() == 0 {
		return 0
	}

	// Binary search for the first timestamp > endTime.
	return searchAfter(stats.timestamps, endTime)
}

// RecordObservationTime records that an observation occurred at the given timestamp.
// This is used for log observations that may not produce virtual metrics but still
// need to appear in DataTimestamps for replay fidelity.
func (s *timeSeriesStorage) RecordObservationTime(timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observationTimestamps[timestamp] = struct{}{}
}

// WriteGeneration returns a counter that increments on every Add call
// (including same-bucket merges). Detectors use this to detect value
// changes that don't create new buckets.
func (s *timeSeriesStorage) WriteGeneration(key observer.SeriesKey) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	k := seriesKey(key.Namespace, key.Name, key.Tags)
	if stats, ok := s.series[k]; ok {
		return stats.writeGeneration
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

// GetSeriesRange returns points within a time range (start, end].
// Start is exclusive, end is inclusive. Use start=0 to read from the beginning.
// Uses binary search on the timestamps column for O(log N) range lookup.
func (s *timeSeriesStorage) GetSeriesRange(key observer.SeriesKey, start, end int64, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	internalKey := seriesKey(key.Namespace, key.Name, key.Tags)
	stats := s.series[internalKey]
	if stats == nil {
		return nil
	}

	// Binary search: find first index where timestamp > start
	lo := searchAfter(stats.timestamps, start)
	// Binary search: find first index where timestamp > end
	hi := searchAfter(stats.timestamps, end)

	// Range is [lo, hi) in the arrays, corresponding to (start, end] in time.
	resultLen := hi - lo
	points := make([]observer.Point, resultLen)

	// For aggregates that map directly to a column, avoid per-point switch.
	switch agg {
	case AggregateSum:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.timestamps[lo+i],
				Value:     stats.sums[lo+i],
			}
		}
	case AggregateMin:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.timestamps[lo+i],
				Value:     stats.mins[lo+i],
			}
		}
	case AggregateMax:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.timestamps[lo+i],
				Value:     stats.maxes[lo+i],
			}
		}
	case AggregateCount:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.timestamps[lo+i],
				Value:     float64(stats.counts[lo+i]),
			}
		}
	default: // AggregateAverage and any unknown
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.timestamps[lo+i],
				Value:     stats.aggregateAt(lo+i, agg),
			}
		}
	}

	return &observer.Series{
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
		Points:    points,
	}
}
