// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// StorageConfig holds tunable parameters for timeSeriesStorage.
type StorageConfig struct {
	// MaxSeries caps live series; when exceeded on Advance, series are evicted
	// until count drops to MaxSeries*(1-EvictionFloorRatio). 0 disables eviction.
	MaxSeries int

	// EvictionFloorRatio controls how far below MaxSeries eviction drains.
	// e.g. 0.1 → drain to 90% of cap, creating a 10% hysteresis band.
	EvictionFloorRatio float64

	// PointRetentionSecs is how long data points are kept per series.
	// Points older than (latest timestamp - PointRetentionSecs) are trimmed
	// on each Add. 0 disables trimming.
	PointRetentionSecs int64

	// MaxCorrelations caps how many unique correlation patterns are retained in
	// the engine's accumulated-correlations map. 0 uses the built-in default
	// (500). -1 disables the cap entirely (suitable for testbench replay where
	// all patterns must be visible regardless of scenario length).
	// Only meaningful when TrackCorrelationHistory is true.
	MaxCorrelations int

	// TrackCorrelationHistory enables the engine's accumulated-correlations map
	// (accumulateCorrelations / AccumulatedCorrelations / CorrelationHistory).
	// Default false — live production mode never reads this map, so the map
	// write + eviction scan on every Advance is avoided. The testbench sets
	// this to true alongside MaxCorrelations=-1 to retain the full history for
	// replay analysis.
	TrackCorrelationHistory bool
}

// DefaultStorageConfig returns the hard-coded production defaults.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		MaxSeries:          storageMaxSeries,
		EvictionFloorRatio: storageEvictionBandRatio,
		PointRetentionSecs: storagePointRetentionSecs,
		// TrackCorrelationHistory defaults to false: live agent incurs no overhead.
	}
}

const (
	// storageMaxSeries is the default cap on live series in storage.
	// Eviction fires when exceeded, draining down to storageEvictionTarget.
	storageMaxSeries = 50_000

	// storageEvictionBandRatio controls how far below the cap eviction drains.
	storageEvictionBandRatio = 0.5

	// storagePointRetentionSecs is the default point retention window.
	// Points older than (latest_ts - 120s) are trimmed on each Add.
	storagePointRetentionSecs = 120
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	cfg    StorageConfig
	mu     sync.RWMutex
	series map[uint64]*seriesStats // keyed by seriesKeyHash; no string retained per entry

	// observationTimestamps tracks all timestamps where observations occurred,
	// even if no metric series was written for that timestamp.
	observationTimestamps map[int64]struct{}

	// Compact numeric IDs for O(1) lookups and API responses.
	// seriesIDStats[ref] is the live *seriesStats (nil when the slot is retired).
	seriesIDStats []*seriesStats // numeric ID → *seriesStats (index = ID)

	// Global generation for the series catalog; increments only when a new
	// series key is created, not on every write to an existing series.
	seriesGen uint64

	// tagIntern maps a fnv64a hash of a series' sorted tag set to the canonical
	// []string slice shared by all series with that tag combination, plus a
	// reference count. When the count drops to zero on eviction the entry is
	// deleted. Protected by s.mu (write lock).
	tagIntern map[uint64]*tagInternEntry

	// Drop accounting for invalid/unsafe input values.
	droppedNonFinite int64
	droppedExtreme   int64
	droppedByMetric  map[string]int64
	sampledDrops     map[string]int
}

// tagInternEntry is the value stored in timeSeriesStorage.tagIntern.
// tags is the canonical []string shared by all series with the same tag set.
// count is the number of live series currently referencing it.
type tagInternEntry struct {
	tags  []string
	count int
}

// seriesStats contains accumulated statistics for a time series (internal).
// Data is stored in columnar layout: parallel arrays indexed by point position.
// Timestamps are stored in sorted order, enabling binary search for range queries.
type seriesStats struct {
	Namespace string
	Name      string
	Tags      []string
	tagsHash  uint64                  // fnv64a hash of Tags; 0 means not interned
	ref       observer.SeriesRef      // compact numeric ID assigned on creation
	context   *observer.MetricContext // optional; set by extractors for anomaly enrichment

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

// sampleCount returns the total number of samples for a series.
// A point can contain multiple samples if it is aggregated.
func (s *seriesStats) sampleCount() int64 {
	count := int64(0)
	for _, c := range s.counts {
		count += c
	}
	return count
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

// searchAfter returns the index of the first timestamp > value using binary search.
func searchAfter(timestamps []int64, value int64) int {
	return sort.Search(len(timestamps), func(i int) bool {
		return timestamps[i] > value
	})
}

// newTimeSeriesStorage creates a new time series storage with default config.
func newTimeSeriesStorage() *timeSeriesStorage {
	return newTimeSeriesStorageWith(DefaultStorageConfig())
}

// newTimeSeriesStorageWith creates a new time series storage with explicit config.
func newTimeSeriesStorageWith(cfg StorageConfig) *timeSeriesStorage {
	return &timeSeriesStorage{
		cfg:                   cfg,
		series:                make(map[uint64]*seriesStats),
		observationTimestamps: make(map[int64]struct{}),
		tagIntern:             make(map[uint64]*tagInternEntry),
		droppedByMetric:       make(map[string]int64),
		sampledDrops:          make(map[string]int),
	}
}

// AddResult bundles the outputs of timeSeriesStorage.Add.
type AddResult struct {
	// IsNew is true if this Add created a brand-new series (cardinality +1).
	IsNew bool
	// Ref is the SeriesRef assigned to this point's series.
	// -1 when the point is dropped (non-finite or sentinel values).
	Ref observer.SeriesRef
}

// Add inserts a (namespace, name, value, timestamp, tags) point into storage.
// Invalid values are dropped at ingest with accounting and sampled logging.
// Timestamps are maintained in sorted order so replay and live ingestion remain
// correct even when data arrives out of order.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) AddResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if math.IsInf(value, 0) || math.IsNaN(value) {
		s.recordDroppedValue("non_finite", namespace, name, value, timestamp, tags)
		return AddResult{Ref: -1}
	}
	// Guard against known finite sentinel values (MaxFloat64 used as "unlimited")
	// that overflow downstream aggregation math when summed.
	if value == math.MaxFloat64 || value == -math.MaxFloat64 {
		s.recordDroppedValue("extreme", namespace, name, value, timestamp, tags)
		return AddResult{Ref: -1}
	}
	h := seriesKeyHash(namespace, name, tags)
	// Skip the alloc when tags are already sorted. Both ingest paths (real metrics
	// via prepareMetricIngest and virtual metrics via IngestLog) canonicalize before
	// calling Add, so this fast path is hit on every normal call.
	var canonTags []string
	if tagsSorted(tags) {
		canonTags = tags
	} else {
		canonTags = canonicalizeTags(tags)
	}

	stats, exists := s.series[h]
	// Collision guard: verify full identity (namespace + name + sorted tags).
	if exists && (stats.Namespace != namespace || stats.Name != name || !tagsEqual(stats.Tags, canonTags)) {
		// Hash collision — extremely rare with FNV-64a (~10^-14 at 1000 series).
		pkglog.Warnf("[observer] seriesKeyHash collision h=%d: incumbent={%s,%s} new={%s,%s}",
			h, stats.Namespace, stats.Name, namespace, name)
		exists = false
		for _, st := range s.seriesIDStats {
			if st != nil && st.Namespace == namespace && st.Name == name && tagsEqual(st.Tags, canonTags) {
				stats = st
				exists = true
				break
			}
		}
	}
	if !exists {
		// Only intern on new series creation so the ref count tracks exactly
		// the number of live series holding the canonical slice.
		canonical, th := s.internTags(tags)
		id := observer.SeriesRef(len(s.seriesIDStats))
		stats = &seriesStats{
			Namespace: namespace,
			Name:      name,
			Tags:      canonical,
			tagsHash:  th,
			ref:       id,
		}
		// Only claim the hash slot when empty to avoid displacing an existing
		// collision-displaced series.
		if _, occupied := s.series[h]; !occupied {
			s.series[h] = stats
		}
		s.seriesIDStats = append(s.seriesIDStats, stats)
		s.seriesGen++
	}
	res := AddResult{IsNew: !exists, Ref: stats.ref}
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
		return res
	}

	stats.timestamps = insertInt64(stats.timestamps, idx, bucket)
	stats.sums = insertFloat64(stats.sums, idx, value)
	stats.counts = insertInt64(stats.counts, idx, 1)
	stats.mins = insertFloat64(stats.mins, idx, value)
	stats.maxes = insertFloat64(stats.maxes, idx, value)

	if s.cfg.PointRetentionSecs > 0 {
		// Trim points outside the retention window. Use the series' latest
		// timestamp (not the incoming bucket) so that backfilled/out-of-order
		// points don't shift the cutoff backwards and over-retain stale data.
		latestTS := stats.timestamps[len(stats.timestamps)-1]
		if trim := searchAfter(stats.timestamps, latestTS-s.cfg.PointRetentionSecs-1); trim > 0 {
			stats.timestamps = trimFront(stats.timestamps, trim)
			stats.sums = trimFront(stats.sums, trim)
			stats.counts = trimFront(stats.counts, trim)
			stats.mins = trimFront(stats.mins, trim)
			stats.maxes = trimFront(stats.maxes, trim)
		}
	}
	return res
}

// trimFront removes the first n elements from s in-place, reusing the backing
// array to avoid allocation. Used to enforce the point retention window.
func trimFront[T any](s []T, n int) []T {
	keep := len(s) - n
	copy(s, s[n:])
	return s[:keep]
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
		pkglog.Warnf("[observer] dropped %s metric value namespace=%q metric=%q value=%g ts=%d tags=%v sample=%d",
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
		// Exact match with tags.
		stats := s.series[seriesKeyHash(namespace, name, tags)]
		if stats == nil || stats.Namespace != namespace || stats.Name != name {
			return nil
		}
		series := stats.toSeries(agg)
		return &series
	}

	// tags is nil: find first series matching namespace and name (ignoring tags).
	for _, stats := range s.seriesIDStats {
		if stats != nil && stats.Namespace == namespace && stats.Name == name {
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

	stats := s.series[seriesKeyHash(namespace, name, tags)]
	if stats == nil || stats.Namespace != namespace || stats.Name != name {
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
	for _, stats := range s.seriesIDStats {
		if stats != nil {
			seen[stats.Namespace] = struct{}{}
		}
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
	for _, stats := range s.seriesIDStats {
		if stats != nil && stats.Namespace == namespace {
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

	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
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
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
		if n := stats.pointCount(); n > 0 {
			if t := stats.timestamps[n-1]; t > max {
				max = t
			}
		}
	}
	return max
}

// seriesKey creates a unique key for a series.
//
// The result has the form "namespace|name|tag1,tag2,...". This function is on
// the hot path for log ingestion and detector loops, so we build the key with
// a single growth via strings.Builder to avoid the chained `+` and intermediate
// joinTags allocations that the naive form produces.
func seriesKey(namespace, name string, tags []string) string {
	if len(tags) > 1 && !tagsSorted(tags) {
		tags = canonicalizeTags(tags)
	}
	// Pre-compute exact length: namespace + '|' + name + '|' + joined(tags).
	n := len(namespace) + 1 + len(name) + 1
	for i, t := range tags {
		if i > 0 {
			n++ // ',' separator
		}
		n += len(t)
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(namespace)
	b.WriteByte('|')
	b.WriteString(name)
	b.WriteByte('|')
	for i, t := range tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(t)
	}
	return b.String()
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

// tagsEqual reports whether two sorted tag slices are identical.
func tagsEqual(a, b []string) bool {
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

// tagInternMaxSize caps the number of unique tag-set entries in the intern
// pool. New combinations beyond the cap are used as-is (no sharing, no pool
// growth); hits on already-interned combinations still return the canonical
// slice. Matches the default for dogstatsd_string_interner_size.
const tagInternMaxSize = 4096

// hashTags computes a fnv64a hash over sorted tags without constructing the
// joined string. Distinct from seriesKeyHash (which includes namespace+name).
// Returns 0 only for empty input; remaps the rare zero hash to 1 as sentinel.
func hashTags(tags []string) uint64 {
	if len(tags) == 0 {
		return 0
	}
	h := fnvOffsetBasis64
	for i, t := range tags {
		if i > 0 {
			h ^= uint64(',')
			h *= fnvPrime64
		}
		for j := 0; j < len(t); j++ {
			h ^= uint64(t[j])
			h *= fnvPrime64
		}
	}
	if h == 0 {
		h = 1
	}
	return h
}

// internTags sorts tags (if needed), hashes, and either returns the canonical
// []string from the pool (incrementing its ref count) or inserts a new entry.
// Returns the canonical slice and its hash. Hash 0 means not interned (cap or
// collision). Must be called with s.mu write-locked.
func (s *timeSeriesStorage) internTags(tags []string) ([]string, uint64) {
	if len(tags) == 0 {
		return nil, 0
	}
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	if len(sorted) > 1 && !tagsSorted(sorted) {
		sort.Strings(sorted)
	}
	th := hashTags(sorted)
	if entry, ok := s.tagIntern[th]; ok {
		if tagsEqual(entry.tags, sorted) {
			entry.count++
			return entry.tags, th
		}
		// Hash collision — skip interning.
		return sorted, 0
	}
	if len(s.tagIntern) >= tagInternMaxSize {
		return sorted, 0
	}
	entry := &tagInternEntry{tags: sorted, count: 1}
	s.tagIntern[th] = entry
	return sorted, th
}

// releaseTagIntern decrements the ref count for the intern entry at th and
// deletes it when count reaches zero. No-op when th is 0. Must be called with
// s.mu write-locked.
func (s *timeSeriesStorage) releaseTagIntern(th uint64) {
	if th == 0 {
		return
	}
	if entry, ok := s.tagIntern[th]; ok {
		entry.count--
		if entry.count == 0 {
			delete(s.tagIntern, th)
		}
	}
}

// TagInternedCount returns the number of unique tag-set entries in the intern
// pool. Useful for telemetry and tests.
func (s *timeSeriesStorage) TagInternedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tagIntern)
}

// seriesKeyHash computes FNV-1a over namespace|name|tag1,tag2,... without
// allocating a string. Produces the same value as fnv64aString(seriesKey(...)).
func seriesKeyHash(namespace, name string, tags []string) uint64 {
	if len(tags) > 1 && !tagsSorted(tags) {
		tags = canonicalizeTags(tags)
	}
	h := fnv64aString(namespace)
	h = fnv64aMix(h, name)
	h ^= uint64('|')
	h *= fnvPrime64
	for i, t := range tags {
		if i > 0 {
			h ^= uint64(',')
			h *= fnvPrime64
		}
		for j := 0; j < len(t); j++ {
			h ^= uint64(t[j])
			h *= fnvPrime64
		}
	}
	return h
}

// resolveByID returns the seriesStats for a numeric series ID.
// Returns nil for out-of-range IDs. Caller must hold s.mu (read or write).
func (s *timeSeriesStorage) resolveByID(ref observer.SeriesRef) *seriesStats {
	if ref < 0 || int(ref) >= len(s.seriesIDStats) {
		return nil
	}
	return s.seriesIDStats[ref]
}

// FindRefsByHashes returns the SeriesRef for each hash present in storage.
// Uses the existing s.series hash map for O(1) per lookup; hashes with no
// matching series are silently skipped.
func (s *timeSeriesStorage) FindRefsByHashes(hashes map[uint64]struct{}) []observer.SeriesRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	refs := make([]observer.SeriesRef, 0, len(hashes))
	for h := range hashes {
		if stats := s.series[h]; stats != nil {
			refs = append(refs, stats.ref)
		}
	}
	return refs
}

// GetSeriesMeta returns the metadata for a series by its numeric ref.
// Returns nil if the ref is out of range.
func (s *timeSeriesStorage) GetSeriesMeta(ref observer.SeriesRef) *observer.SeriesMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ss := s.resolveByID(ref)
	if ss == nil {
		return nil
	}
	return &observer.SeriesMeta{
		Ref:       ref,
		Namespace: ss.Namespace,
		Name:      ss.Name,
		Tags:      ss.Tags,
	}
}

// seriesMeta is lightweight series metadata including point count,
// used for API listing without materializing point data.
type seriesMeta struct {
	Ref        observer.SeriesRef // compact numeric ref
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
	for _, stats := range s.seriesIDStats {
		if stats != nil && stats.Namespace == namespace {
			result = append(result, seriesMeta{
				Ref:        stats.ref,
				Namespace:  stats.Namespace,
				Name:       stats.Name,
				Tags:       copyTags(stats.Tags),
				PointCount: stats.pointCount(),
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Ref != result[j].Ref {
			return result[i].Ref < result[j].Ref
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
func (s *timeSeriesStorage) GetSeriesByNumericID(ref observer.SeriesRef, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
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

	result := make([]seriesCompact, 0, len(s.seriesIDStats))
	for _, st := range s.seriesIDStats {
		if st == nil {
			continue
		}
		result = append(result, seriesCompact{
			Namespace: st.Namespace,
			Name:      st.Name,
			Tags:      st.Tags,
		})
	}
	return result
}

// ---------------------------------------------------------------------------
// Inline FNV-1a (64-bit) — zero alloc on the hot path.
// Produces identical output to stdlib hash/fnv.New64a().
// ---------------------------------------------------------------------------

const (
	fnvOffsetBasis64 = uint64(14695981039346656037)
	fnvPrime64       = uint64(1099511628211)
)

// fnv64aString computes FNV-1a over a string without allocating a hasher or
// converting to []byte.
func fnv64aString(s string) uint64 {
	h := fnvOffsetBasis64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

// fnv64aMix folds an additional string into an existing FNV-1a hash, separated
// by '|'. Useful for hashing multiple fields without concatenating them first.
func fnv64aMix(h uint64, s string) uint64 {
	h ^= uint64('|')
	h *= fnvPrime64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
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
	for _, st := range s.seriesIDStats {
		if st == nil {
			continue
		}
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
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
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

// SeriesGeneration returns a counter that increments whenever the series
// catalog changes — either when a new series key is created or when an
// existing key is removed via RemoveSeriesByRefs or RemoveSeriesByMetricName.
// Callers can use this to safely cache ListSeries results.
func (s *timeSeriesStorage) SeriesGeneration() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.seriesGen
}

// RemoveSeriesByRefs deletes series by their compact numeric refs. Each removed
// series has its seriesIDStats slot set to nil (ref is never reused) and its
// hash slot deleted from s.series. Returns the refs actually freed; out-of-range
// or already-nil refs are silently skipped. seriesGen is bumped iff at least
// one series was removed so cached ListSeries results are invalidated.
//
// Callers use the returned refs to fan out per-series teardown to detector
// state keyed by SeriesRef (BOCPD posteriors, ScanMW/ScanWelch segment buffers,
// seriesDetectorAdapter visible-count maps, etc.).
func (s *timeSeriesStorage) RemoveSeriesByRefs(refs []observer.SeriesRef) []observer.SeriesRef {
	if len(refs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed []observer.SeriesRef
	for _, ref := range refs {
		if ref < 0 || int(ref) >= len(s.seriesIDStats) {
			continue
		}
		stats := s.seriesIDStats[ref]
		if stats == nil {
			continue
		}
		s.releaseTagIntern(stats.tagsHash)
		h := seriesKeyHash(stats.Namespace, stats.Name, stats.Tags)
		delete(s.series, h)
		s.seriesIDStats[ref] = nil
		removed = append(removed, ref)
	}
	if len(removed) > 0 {
		s.seriesGen++
	}
	return removed
}

// SetContext stores a MetricContext on the series identified by ref.
// No-op when ref is out of range or the series has been removed.
func (s *timeSeriesStorage) SetContext(ref observer.SeriesRef, ctx *observer.MetricContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stats := s.resolveByID(ref); stats != nil {
		stats.context = ctx
	}
}

// GetContext returns the MetricContext stored on the series identified by ref.
// Returns nil when ref is out of range, the series has been removed, or no
// context was set.
func (s *timeSeriesStorage) GetContext(ref observer.SeriesRef) *observer.MetricContext {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stats := s.resolveByID(ref); stats != nil {
		return stats.context
	}
	return nil
}

// RemoveSeriesByMetricName removes all series in the given namespace whose Name
// matches name. Used when an extractor GC/LRU evicts a pattern cluster — the
// cluster identity (namespace + metric name) is deterministic, so we can clean
// up all tag variants without needing to track individual SeriesRefs.
// Returns the freed refs for fan-out to detectors.
func (s *timeSeriesStorage) RemoveSeriesByMetricName(namespace, name string) []observer.SeriesRef {
	if name == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed []observer.SeriesRef
	for _, stats := range s.seriesIDStats {
		if stats == nil || stats.Namespace != namespace || stats.Name != name {
			continue
		}
		s.releaseTagIntern(stats.tagsHash)
		h := seriesKeyHash(stats.Namespace, stats.Name, stats.Tags)
		delete(s.series, h)
		s.seriesIDStats[stats.ref] = nil
		removed = append(removed, stats.ref)
	}
	if len(removed) > 0 {
		s.seriesGen++
	}
	return removed
}

// EvictToCapacity evicts the oldest series (by last written timestamp) when
// the live series count exceeds seriesLimit, draining down to target. The band
// between the two thresholds prevents a fan-out on every Advance when the
// count hovers near the cap. Returns the freed SeriesRefs for detector cleanup.
func (s *timeSeriesStorage) EvictToCapacity(seriesLimit, target int) []observer.SeriesRef {
	if seriesLimit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Count first — common case is under the limit, skip allocation entirely.
	count := 0
	for _, st := range s.seriesIDStats {
		if st != nil {
			count++
		}
	}
	if count <= seriesLimit {
		return nil
	}

	type entry struct {
		ref    observer.SeriesRef
		lastTs int64
	}
	candidates := make([]entry, 0, count)
	for _, st := range s.seriesIDStats {
		if st == nil {
			continue
		}
		lastTs := int64(0)
		if n := len(st.timestamps); n > 0 {
			lastTs = st.timestamps[n-1]
		}
		candidates = append(candidates, entry{ref: st.ref, lastTs: lastTs})
	}

	excess := count - target
	if excess <= 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].lastTs < candidates[j].lastTs
	})

	var freed []observer.SeriesRef
	for i := 0; i < excess; i++ {
		st := s.resolveByID(candidates[i].ref)
		if st == nil {
			continue
		}
		h := seriesKeyHash(st.Namespace, st.Name, st.Tags)
		s.releaseTagIntern(st.tagsHash)
		if s.series[h] == st {
			delete(s.series, h)
		}
		s.seriesIDStats[candidates[i].ref] = nil
		freed = append(freed, candidates[i].ref)
	}
	if len(freed) > 0 {
		s.seriesGen++
	}
	return freed
}

// EvictDefault evicts to capacity using the storage's own config.
// The eviction target is MaxSeries*(1-EvictionFloorRatio).
func (s *timeSeriesStorage) EvictDefault() []observer.SeriesRef {
	if s.cfg.MaxSeries <= 0 {
		return nil
	}
	target := s.cfg.MaxSeries - int(float64(s.cfg.MaxSeries)*s.cfg.EvictionFloorRatio)
	return s.EvictToCapacity(s.cfg.MaxSeries, target)
}

// CompactSeriesID translates a full series key to its compact numeric ID string.
// The full key format is "namespace|name:agg|tags" where the storage key is
// "namespace|name|tags" (without the agg suffix). This method strips the agg
// suffix, looks up the numeric ID, and returns "numericID:agg".
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

	// Look up by hash; verify identity to guard against hash collisions.
	stats := s.series[seriesKeyHash(namespace, baseName, tags)]
	if stats == nil || stats.Namespace != namespace || stats.Name != baseName {
		return fullKey
	}

	if aggStr != "" {
		return strconv.Itoa(int(stats.ref)) + ":" + aggStr
	}
	return strconv.Itoa(int(stats.ref))
}

// StorageReader interface implementation

// ListSeries returns metadata for all series matching the filter.
func (s *timeSeriesStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Preallocate to len(s.series): an upper bound under the lock that lets
	// us avoid repeated growslice in the common case where the filter matches
	// most series. Detectors and the adapter call this on every advance, so
	// even after the cache-by-gen optimisations the worst-case cost matters
	// when seriesGen does churn (e.g. cardinality blow-ups in extractors).
	result := make([]observer.SeriesMeta, 0, len(s.series))
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
		if !matchesSeriesFilter(stats, filter) {
			continue
		}
		result = append(result, observer.SeriesMeta{
			Ref:       stats.ref,
			Namespace: stats.Namespace,
			Name:      stats.Name,
			Tags:      stats.Tags,
		})
	}
	return result
}

// ListSeriesRefsInto uses dst as scratch and returns refs for all series
// matching the filter. Previous dst contents are discarded. It is the
// allocation-light detector hot path for callers that only need the stable
// numeric SeriesRef handles.
func (s *timeSeriesStorage) ListSeriesRefsInto(filter observer.SeriesFilter, dst []observer.SeriesRef) []observer.SeriesRef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cap(dst) < len(s.series) {
		dst = make([]observer.SeriesRef, 0, len(s.series))
	} else {
		dst = dst[:0]
	}
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
		if !matchesSeriesFilter(stats, filter) {
			continue
		}
		dst = append(dst, stats.ref)
	}
	return dst
}

// PointCount returns the number of raw data points for a series.
func (s *timeSeriesStorage) PointCount(ref observer.SeriesRef) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats := s.resolveByID(ref); stats != nil {
		return stats.pointCount()
	}
	return 0
}

// TotalSampleCount returns the total number of stored samples across all series,
// excluding series in excludeNamespace (pass "" to include all namespaces).
// A point can contain multiple samples if it is aggregated.
func (s *timeSeriesStorage) TotalSampleCount(excludeNamespace string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := int64(0)
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
		if excludeNamespace != "" && stats.Namespace == excludeNamespace {
			continue
		}
		total += stats.sampleCount()
	}
	return total
}

// TotalSeriesCount returns the number of unique series (name + tag combinations),
// excluding series in excludeNamespace (pass "" to include all namespaces).
func (s *timeSeriesStorage) TotalSeriesCount(excludeNamespace string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := 0
	for _, stats := range s.seriesIDStats {
		if stats == nil {
			continue
		}
		if excludeNamespace != "" && stats.Namespace == excludeNamespace {
			continue
		}
		total++
	}
	return total
}

// PointCountUpTo returns the number of raw data points with timestamp <= endTime.
// Uses binary search since timestamps are sorted.
func (s *timeSeriesStorage) PointCountUpTo(ref observer.SeriesRef, endTime int64) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
	if stats == nil || stats.pointCount() == 0 {
		return 0
	}
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
func (s *timeSeriesStorage) WriteGeneration(ref observer.SeriesRef) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stats := s.resolveByID(ref); stats != nil {
		return stats.writeGeneration
	}
	return 0
}

// BulkSeriesStatus returns the point count (up to endTime) and write generation
// for each ref in a single lock acquisition. This avoids the overhead of
// 2×len(refs) individual RLock/RUnlock calls in hot detector loops.
// Implements bulkStatusReader (metrics_detector_util.go).
func (s *timeSeriesStorage) BulkSeriesStatus(refs []observer.SeriesRef, endTime int64) []seriesStatus {
	result := make([]seriesStatus, len(refs))

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i, ref := range refs {
		stats := s.resolveByID(ref)
		if stats == nil || stats.pointCount() == 0 {
			continue
		}
		result[i] = seriesStatus{
			pointCount:      searchAfter(stats.timestamps, endTime),
			writeGeneration: stats.writeGeneration,
		}
	}
	return result
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

func matchesSeriesFilter(stats *seriesStats, filter observer.SeriesFilter) bool {
	if filter.Namespace != "" {
		if stats.Namespace != filter.Namespace {
			return false
		}
	} else {
		for _, ex := range filter.ExcludeNamespaces {
			if stats.Namespace == ex {
				return false
			}
		}
	}
	if filter.NamePattern != "" && !strings.HasPrefix(stats.Name, filter.NamePattern) {
		return false
	}
	return matchTags(stats.Tags, filter.TagMatchers)
}

// GetSeriesRange returns points within a time range (start, end].
// Start is exclusive, end is inclusive. Use start=0 to read from the beginning.
// Uses binary search on the timestamps column for O(log N) range lookup.
func (s *timeSeriesStorage) GetSeriesRange(ref observer.SeriesRef, start, end int64, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
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

// pointBufPool reuses point buffers across ForEachPoint calls to avoid
// per-call allocation. Each buffer grows to its high-water mark and stays.
var pointBufPool = sync.Pool{
	New: func() any { return &[]observer.Point{} },
}

// ForEachPoint calls fn for every point in the time range (start, end].
// The Series pointer is valid only for the duration of the callback.
// Returns false if the series was not found.
//
// Points are copied under the read lock into a pooled buffer; the callback
// runs outside the lock so callers cannot block writers.
func (s *timeSeriesStorage) ForEachPoint(
	ref observer.SeriesRef, start, end int64, agg Aggregate,
	fn func(*observer.Series, observer.Point),
) bool {
	bufp := pointBufPool.Get().(*[]observer.Point)
	buf := *bufp

	series, buf, ok := s.snapshotRange(ref, start, end, agg, buf)
	if !ok {
		*bufp = buf
		pointBufPool.Put(bufp)
		return false
	}

	for _, p := range buf {
		fn(&series, p)
	}

	*bufp = buf
	pointBufPool.Put(bufp)
	return true
}

// SumRange returns the aggregate total over the time range (start, end] without
// allocating any intermediate slices. It operates directly on the columnar
// data arrays, using binary search to locate the range boundaries.
// Returns 0 if the series is not found or the range is empty.
func (s *timeSeriesStorage) SumRange(ref observer.SeriesRef, start, end int64, agg Aggregate) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}

	lo := searchAfter(stats.timestamps, start)
	hi := searchAfter(stats.timestamps, end)
	if lo >= hi {
		return 0
	}

	var total float64
	switch agg {
	case AggregateSum:
		for _, v := range stats.sums[lo:hi] {
			total += v
		}
	case AggregateCount:
		for _, c := range stats.counts[lo:hi] {
			total += float64(c)
		}
	case AggregateMin:
		for _, v := range stats.mins[lo:hi] {
			total += v
		}
	case AggregateMax:
		for _, v := range stats.maxes[lo:hi] {
			total += v
		}
	default: // AggregateAverage
		for i := lo; i < hi; i++ {
			total += stats.aggregateAt(i, agg)
		}
	}
	return total
}

// snapshotRange copies points for a time range into buf under the read lock.
// Returns the series metadata, the (potentially grown) buffer, and whether the
// series was found.
func (s *timeSeriesStorage) snapshotRange(
	ref observer.SeriesRef, start, end int64, agg Aggregate,
	buf []observer.Point,
) (observer.Series, []observer.Point, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
	if stats == nil {
		return observer.Series{}, buf, false
	}

	lo := searchAfter(stats.timestamps, start)
	hi := searchAfter(stats.timestamps, end)
	n := hi - lo

	if cap(buf) >= n {
		buf = buf[:n]
	} else {
		buf = make([]observer.Point, n)
	}

	for i := 0; i < n; i++ {
		buf[i] = observer.Point{
			Timestamp: stats.timestamps[lo+i],
			Value:     stats.aggregateAt(lo+i, agg),
		}
	}

	return observer.Series{
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
	}, buf, true
}
