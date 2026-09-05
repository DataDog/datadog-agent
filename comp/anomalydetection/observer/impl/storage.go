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

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
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

	// MaxPointsPerSeries is the maximum number of processable points retained
	// for a series. Storage keeps one additional pending scheduler bucket.
	// Zero disables count-based trimming.
	MaxPointsPerSeries int

	// InactiveSeriesTTLSeconds is how long a non-telemetry series may remain
	// inactive before an engine advance evicts it. 0 disables inactivity eviction.
	InactiveSeriesTTLSeconds int64

	// InactiveSeriesCheckIntervalSeconds is the minimum advance-time interval
	// between inactivity scans. 0 disables inactivity eviction.
	InactiveSeriesCheckIntervalSeconds int64

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

	// TrackAnomalyHistory enables full raw anomaly history for replay/debug
	// introspection. Default false: live production keeps only a bounded dedup
	// cache because reporters consume advance-local anomalies directly. The
	// testbench enables this to display every anomaly from a finite replay.
	TrackAnomalyHistory bool
}

// DefaultStorageConfig returns the hard-coded production defaults.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		MaxSeries:                          storageMaxSeries,
		EvictionFloorRatio:                 storageEvictionBandRatio,
		PointRetentionSecs:                 storagePointRetentionSecs,
		InactiveSeriesTTLSeconds:           storageInactiveSeriesTTLSeconds,
		InactiveSeriesCheckIntervalSeconds: storageInactiveSeriesCheckIntervalSeconds,
		// History tracking defaults to false: live agent incurs no replay-only overhead.
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

	// storageInactiveSeriesTTLSeconds is the default inactivity lifetime for
	// non-telemetry series. Inactivity is evaluated against advance timestamps.
	storageInactiveSeriesTTLSeconds = 5 * 60

	// storageInactiveSeriesCheckIntervalSeconds bounds the work done by
	// inactivity scans while keeping eviction deterministic under replay.
	storageInactiveSeriesCheckIntervalSeconds = 5 * 60
)

// timeSeriesStorage is an internal storage for time series data.
type timeSeriesStorage struct {
	cfg    StorageConfig
	mu     sync.RWMutex
	series map[uint64]*seriesStats // keyed by seriesKeyHash; no string retained per entry

	// observationTimestamps tracks all timestamps where observations occurred,
	// even if no metric series was written for that timestamp.
	observationTimestamps map[int64]struct{}

	// Compact numeric IDs for O(1) lookups and API responses. Retired refs are
	// removed from the map and never reused, keeping this index bounded by live
	// series instead of cumulative series churn.
	seriesIDStats map[observer.SeriesRef]*seriesStats
	nextSeriesRef observer.SeriesRef

	// liveSeriesCount is the number of non-telemetry series in the catalog.
	// It is updated under mu whenever a non-telemetry series is added or removed.
	liveSeriesCount int

	// Global generation for the series catalog; increments only when a new
	// series key is created, not on every write to an existing series.
	seriesGen uint64

	// tagIntern maps a fnv64a hash of a series' sorted tag set to the canonical
	// []string slice shared by all series with that tag combination, plus a
	// reference count. When the count drops to zero on eviction the entry is
	// deleted. Protected by s.mu (write lock).
	tagIntern map[uint64]*tagInternEntry
}

// tagInternEntry is the value stored in timeSeriesStorage.tagIntern.
// tags is the canonical []string shared by all series with the same tag set.
// count is the number of live series currently referencing it.
type tagInternEntry struct {
	tags  []string
	count int
}

// pointBucket contains the timestamp and sum for one one-second bucket.
// A count of one is implicit; seriesStats allocates a parallel count vector
// only after multiple samples land in the same bucket.
type pointBucket struct {
	timestamp int64
	sum       float64
}

// bucketCounts holds explicit per-bucket sample counts for series that have
// observed at least one same-second merge. Keeping the slice behind a pointer
// adds one word per series while reducing the common point payload from 24 to
// 16 bytes. Series whose buckets all contain one sample never allocate it.
type bucketCounts struct {
	values []int64
}

// seriesStats contains accumulated statistics for a time series (internal).
// Buckets are stored in timestamp order, enabling binary search for range queries.
type seriesStats struct {
	Namespace string
	Name      string
	Tags      []string
	tagsHash  uint64                  // fnv64a hash of Tags; 0 means not interned
	ref       observer.SeriesRef      // compact numeric ID assigned on creation
	context   *observer.MetricContext // optional; set by extractors for anomaly enrichment
	// supportedAggregations is a bit mask. Zero means all aggregations are
	// supported; materialized log count buckets set only Average because each
	// stored point is already one aggregated window count.
	supportedAggregations uint8
	// retentionOverrideSecs, when positive, replaces the storage-wide point
	// retention for this series. Zero uses the storage default.
	retentionOverrideSecs int64
	// lastActivityTimestamp drives capacity eviction. It normally follows the
	// latest stored timestamp, but producers of synthetic points may override it
	// so generated data does not make an otherwise-idle series look active.
	lastActivityTimestamp int64

	// writeGeneration is per-series and increments on every Add, including
	// same-bucket merges into an existing point.
	writeGeneration int64

	buckets []pointBucket
	counts  *bucketCounts
}

func aggregateMask(agg observer.Aggregate) uint8 {
	return 1 << uint8(agg)
}

// pointCount returns the number of stored points.
func (s *seriesStats) pointCount() int {
	return len(s.buckets)
}

// countAt returns the sample count for bucket i. A nil explicit count vector
// means every bucket contains exactly one sample.
func (s *seriesStats) countAt(i int) int64 {
	if s.counts == nil {
		return 1
	}
	return s.counts.values[i]
}

// incrementCount materializes exact counts on the first same-second merge.
func (s *seriesStats) incrementCount(i int) {
	if s.counts == nil {
		// Most merged series keep receiving samples. A small initial reserve
		// avoids growing the parallel vector on every early bucket without
		// imposing the full retention-window cost on a one-off merge.
		countCapacity := max(8, cap(s.buckets))
		values := make([]int64, len(s.buckets), countCapacity)
		for j := range values {
			values[j] = 1
		}
		s.counts = &bucketCounts{values: values}
	}
	s.counts.values[i]++
}

// insertCount appends the implicit count for a newly inserted bucket when a
// series already has an explicit count vector.
func (s *seriesStats) insertCount(i int) {
	if s.counts == nil {
		return
	}
	values := append(s.counts.values, 0)
	copy(values[i+1:], values[i:])
	values[i] = 1
	s.counts.values = values
}

// trimBuckets removes the oldest n buckets and keeps an explicit count vector
// aligned with the point data.
func (s *seriesStats) trimBuckets(n int) {
	s.buckets = trimFront(s.buckets, n)
	if s.counts != nil {
		s.counts.values = trimFront(s.counts.values, n)
	}
}

// sampleCount returns the total number of samples for a series.
// A point can contain multiple samples if it is aggregated.
func (s *seriesStats) sampleCount() int64 {
	count := int64(0)
	for i := range s.buckets {
		count += s.countAt(i)
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
)

// aggregateAt extracts the specified statistic at index i.
func (s *seriesStats) aggregateAt(i int, agg Aggregate) float64 {
	bucket := s.buckets[i]
	count := s.countAt(i)
	switch agg {
	case AggregateAverage:
		if count == 0 {
			return 0
		}
		return bucket.sum / float64(count)
	case AggregateSum:
		return bucket.sum
	case AggregateCount:
		return float64(count)
	default:
		return 0
	}
}

// toSeries converts internal stats to the simplified Series for analyses.
func (s *seriesStats) toSeries(agg Aggregate) observer.Series {
	n := s.pointCount()
	points := make([]observer.Point, n)
	for i := 0; i < n; i++ {
		points[i] = observer.Point{
			Timestamp: s.buckets[i].timestamp,
			Value:     s.aggregateAt(i, agg),
		}
	}
	return observer.Series{
		Namespace: s.Namespace,
		Name:      s.Name,
		Tags:      s.Tags,
		Points:    points,
	}
}

// searchAfter returns the index of the first bucket timestamp > value using binary search.
func searchAfter(buckets []pointBucket, value int64) int {
	return sort.Search(len(buckets), func(i int) bool {
		return buckets[i].timestamp > value
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
		seriesIDStats:         make(map[observer.SeriesRef]*seriesStats),
		observationTimestamps: make(map[int64]struct{}),
		tagIntern:             make(map[uint64]*tagInternEntry),
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
// Invalid values are dropped at ingest.
// Timestamps are maintained in sorted order so replay and live ingestion remain
// correct even when data arrives out of order.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) AddResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if math.IsInf(value, 0) || math.IsNaN(value) {
		return AddResult{Ref: -1}
	}
	// Guard against known finite sentinel values (MaxFloat64 used as "unlimited")
	// that overflow downstream aggregation math when summed.
	if value == math.MaxFloat64 || value == -math.MaxFloat64 {
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
		logging.Warnf("seriesKeyHash collision h=%d: incumbent={%s,%s} new={%s,%s}",
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
		id := s.nextSeriesRef
		s.nextSeriesRef++
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
		s.seriesIDStats[id] = stats
		if namespace != observer.TelemetryNamespace {
			s.liveSeriesCount++
		}
		s.seriesGen++
	}
	res := AddResult{IsNew: !exists, Ref: stats.ref}
	stats.writeGeneration++
	if len(stats.buckets) == 0 || timestamp > stats.lastActivityTimestamp {
		stats.lastActivityTimestamp = timestamp
	}

	// Bucket by second.
	bucket := timestamp

	// Binary search for the bucket in the sorted bucket array.
	idx := sort.Search(len(stats.buckets), func(i int) bool {
		return stats.buckets[i].timestamp >= bucket
	})

	if idx < len(stats.buckets) && stats.buckets[idx].timestamp == bucket {
		// Update existing bucket in-place.
		stats.buckets[idx].sum += value
		stats.incrementCount(idx)
		return res
	}

	stats.buckets = insertBucket(stats.buckets, idx, pointBucket{
		timestamp: bucket,
		sum:       value,
	})
	stats.insertCount(idx)

	retentionSecs := s.cfg.PointRetentionSecs
	if stats.retentionOverrideSecs > 0 {
		retentionSecs = stats.retentionOverrideSecs
	}
	if retentionSecs > 0 {
		// Trim points outside the retention window. Use the series' latest
		// timestamp (not the incoming bucket) so that backfilled/out-of-order
		// points don't shift the cutoff backwards and over-retain stale data.
		latestTS := stats.buckets[len(stats.buckets)-1].timestamp
		if trim := searchAfter(stats.buckets, latestTS-retentionSecs-1); trim > 0 {
			stats.trimBuckets(trim)
		}
	}
	if s.cfg.MaxPointsPerSeries > 0 {
		physicalCapacity := s.cfg.MaxPointsPerSeries + 1
		if trim := len(stats.buckets) - physicalCapacity; trim > 0 {
			stats.trimBuckets(trim)
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

// insertBucket inserts v at position idx in s, maintaining timestamp order.
func insertBucket(s []pointBucket, idx int, v pointBucket) []pointBucket {
	s = append(s, pointBucket{})
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
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
	startIdx := searchAfter(stats.buckets, since)

	n := stats.pointCount()
	points := make([]observer.Point, 0, n-startIdx)
	for i := startIdx; i < n; i++ {
		points = append(points, observer.Point{
			Timestamp: stats.buckets[i].timestamp,
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
		firstIdx := searchAfter(stats.buckets, 0)
		if firstIdx >= n {
			continue
		}
		first := stats.buckets[firstIdx].timestamp
		last := stats.buckets[n-1].timestamp
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
			if t := stats.buckets[n-1].timestamp; t > max {
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
// Returns nil for unknown or retired IDs. Caller must hold s.mu (read or write).
func (s *timeSeriesStorage) resolveByID(ref observer.SeriesRef) *seriesStats {
	if ref < 0 {
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
				Timestamp: st.buckets[i].timestamp,
				Sum:       st.buckets[i].sum,
				Count:     st.countAt(i),
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
		for _, bucket := range stats.buckets {
			seen[bucket.timestamp] = struct{}{}
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
// series is deleted from seriesIDStats (ref is never reused) and its hash slot
// is deleted from s.series. Returns the refs actually freed; unknown or already
// removed refs are silently skipped. seriesGen is bumped iff at least
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
		stats := s.resolveByID(ref)
		if stats == nil {
			continue
		}
		if s.removeSeries(stats) {
			removed = append(removed, ref)
		}
	}
	if len(removed) > 0 {
		s.seriesGen++
	}
	return removed
}

// removeSeries removes a live series from every catalog index and cardinality
// counter. The caller must hold s.mu for writing.
func (s *timeSeriesStorage) removeSeries(stats *seriesStats) bool {
	if stats == nil || stats.ref < 0 || s.seriesIDStats[stats.ref] != stats {
		return false
	}
	s.releaseTagIntern(stats.tagsHash)
	h := seriesKeyHash(stats.Namespace, stats.Name, stats.Tags)
	if s.series[h] == stats {
		delete(s.series, h)
	}
	delete(s.seriesIDStats, stats.ref)
	if stats.Namespace != observer.TelemetryNamespace {
		s.liveSeriesCount--
	}
	return true
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

// SetSupportedAggregations limits which interpretations detectors should use
// for a series. An empty list restores the default of supporting all.
func (s *timeSeriesStorage) SetSupportedAggregations(ref observer.SeriesRef, aggregations ...observer.Aggregate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.resolveByID(ref)
	if stats == nil {
		return
	}
	var mask uint8
	for _, agg := range aggregations {
		mask |= aggregateMask(agg)
	}
	stats.supportedAggregations = mask
}

// SupportsAggregate implements the optional detector aggregate policy.
func (s *timeSeriesStorage) SupportsAggregate(ref observer.SeriesRef, agg observer.Aggregate) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := s.resolveByID(ref)
	if stats == nil || stats.supportedAggregations == 0 {
		return true
	}
	return stats.supportedAggregations&aggregateMask(agg) != 0
}

// SetSeriesRetention overrides point retention for one series. Zero restores
// the storage-wide default.
func (s *timeSeriesStorage) SetSeriesRetention(ref observer.SeriesRef, retentionSecs int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stats := s.resolveByID(ref); stats != nil {
		stats.retentionOverrideSecs = max(retentionSecs, 0)
	}
}

// pointRetentionForSeries returns the effective point-retention window for a
// series. Missing series use the storage-wide value, which also provides the
// fallback for anomalies that do not have a storage-backed source.
func (s *timeSeriesStorage) pointRetentionForSeries(ref observer.SeriesRef) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if stats := s.resolveByID(ref); stats != nil && stats.retentionOverrideSecs > 0 {
		return stats.retentionOverrideSecs
	}
	return s.cfg.PointRetentionSecs
}

// SetSeriesActivityTimestamp overrides the timestamp used to rank a series for
// capacity eviction. Materialized log-count series use the last real log time
// so synthetic zero buckets do not keep an idle series artificially hot.
func (s *timeSeriesStorage) SetSeriesActivityTimestamp(ref observer.SeriesRef, timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stats := s.resolveByID(ref); stats != nil {
		stats.lastActivityTimestamp = timestamp
	}
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
		if s.removeSeries(stats) {
			removed = append(removed, stats.ref)
		}
	}
	if len(removed) > 0 {
		s.seriesGen++
	}
	return removed
}

// EvictToCapacity evicts the oldest series (by last activity timestamp) when
// the live series count exceeds seriesLimit, draining down to target. The band
// between the two thresholds prevents a fan-out on every Advance when the
// count hovers near the cap. Returns the freed SeriesRefs for detector cleanup.
func (s *timeSeriesStorage) EvictToCapacity(seriesLimit, target int) []observer.SeriesRef {
	if seriesLimit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Common case is under the limit, skip allocation entirely.
	count := s.liveSeriesCount
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
		candidates = append(candidates, entry{ref: st.ref, lastTs: st.lastActivityTimestamp})
	}

	excess := count - target
	if excess <= 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].lastTs != candidates[j].lastTs {
			return candidates[i].lastTs < candidates[j].lastTs
		}
		return candidates[i].ref < candidates[j].ref
	})

	var freed []observer.SeriesRef
	for i := 0; i < excess; i++ {
		st := s.resolveByID(candidates[i].ref)
		if st == nil {
			continue
		}
		if s.removeSeries(st) {
			freed = append(freed, candidates[i].ref)
		}
	}
	if len(freed) > 0 {
		s.seriesGen++
	}
	return freed
}

// EvictInactiveBefore removes non-telemetry series whose last activity is at
// or before cutoff. The caller supplies a data-time cutoff so eviction is
// deterministic in both live operation and replay.
func (s *timeSeriesStorage) EvictInactiveBefore(cutoff int64) []observer.SeriesRef {
	s.mu.Lock()
	defer s.mu.Unlock()

	var freed []observer.SeriesRef
	for _, stats := range s.seriesIDStats {
		if stats == nil || stats.Namespace == observer.TelemetryNamespace || stats.lastActivityTimestamp > cutoff {
			continue
		}
		if s.removeSeries(stats) {
			freed = append(freed, stats.ref)
		}
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

// TotalSeriesCount returns the number of unique non-telemetry series (name +
// tag combinations).
func (s *timeSeriesStorage) TotalSeriesCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liveSeriesCount
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
	return searchAfter(stats.buckets, endTime)
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
			pointCount:      searchAfter(stats.buckets, endTime),
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
// Uses binary search on the ordered buckets for O(log N) range lookup.
func (s *timeSeriesStorage) GetSeriesRange(ref observer.SeriesRef, start, end int64, agg Aggregate) *observer.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
	if stats == nil {
		return nil
	}

	// Binary search: find first index where timestamp > start
	lo := searchAfter(stats.buckets, start)
	// Binary search: find first index where timestamp > end
	hi := searchAfter(stats.buckets, end)

	// Range is [lo, hi) in the bucket slice, corresponding to (start, end] in time.
	resultLen := hi - lo
	points := make([]observer.Point, resultLen)

	// For aggregates that map directly to a column, avoid per-point switch.
	switch agg {
	case AggregateSum:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.buckets[lo+i].timestamp,
				Value:     stats.buckets[lo+i].sum,
			}
		}
	case AggregateCount:
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.buckets[lo+i].timestamp,
				Value:     float64(stats.countAt(lo + i)),
			}
		}
	default: // AggregateAverage and any unknown
		for i := 0; i < resultLen; i++ {
			points[i] = observer.Point{
				Timestamp: stats.buckets[lo+i].timestamp,
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

// ForEachLastPoints calls fn for up to n of the newest points with timestamp
// <= end. Like ForEachPoint, it snapshots points under the read lock and runs
// the callback outside the lock. The Series pointer is valid only during fn.
func (s *timeSeriesStorage) ForEachLastPoints(
	ref observer.SeriesRef, end int64, n int, agg Aggregate,
	fn func(*observer.Series, observer.Point),
) bool {
	if n <= 0 {
		return false
	}

	bufp := pointBufPool.Get().(*[]observer.Point)
	buf := *bufp

	s.mu.RLock()
	stats := s.resolveByID(ref)
	if stats == nil {
		s.mu.RUnlock()
		*bufp = buf
		pointBufPool.Put(bufp)
		return false
	}
	endIndex := searchAfter(stats.buckets, end)
	startIndex := max(0, endIndex-n)
	series := observer.Series{Namespace: stats.Namespace, Name: stats.Name, Tags: stats.Tags}
	buf = snapshotPoints(stats, startIndex, endIndex, agg, buf)
	s.mu.RUnlock()

	for _, p := range buf {
		fn(&series, p)
	}

	*bufp = buf
	pointBufPool.Put(bufp)
	return true
}

// SumRange returns the aggregate total over the time range (start, end] without
// allocating any intermediate slices. It operates directly on the bucket
// slice, using binary search to locate the range boundaries.
// Returns 0 if the series is not found or the range is empty.
func (s *timeSeriesStorage) SumRange(ref observer.SeriesRef, start, end int64, agg Aggregate) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}

	lo := searchAfter(stats.buckets, start)
	hi := searchAfter(stats.buckets, end)
	if lo >= hi {
		return 0
	}

	var total float64
	switch agg {
	case AggregateSum:
		for _, bucket := range stats.buckets[lo:hi] {
			total += bucket.sum
		}
	case AggregateCount:
		for i := lo; i < hi; i++ {
			total += float64(stats.countAt(i))
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

	lo := searchAfter(stats.buckets, start)
	hi := searchAfter(stats.buckets, end)
	buf = snapshotPoints(stats, lo, hi, agg, buf)

	return observer.Series{
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
	}, buf, true
}

// snapshotPoints copies [lo, hi) from stats into buf. The caller must hold
// stats' storage read lock.
func snapshotPoints(stats *seriesStats, lo, hi int, agg Aggregate, buf []observer.Point) []observer.Point {
	n := hi - lo
	if cap(buf) >= n {
		buf = buf[:n]
	} else {
		buf = make([]observer.Point, n)
	}
	for i := range buf {
		buf[i] = observer.Point{
			Timestamp: stats.buckets[lo+i].timestamp,
			Value:     stats.aggregateAt(lo+i, agg),
		}
	}
	return buf
}
