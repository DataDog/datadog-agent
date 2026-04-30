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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// numStorageShards is the number of independently-locked shards the storage
// is partitioned into. A series identity hashes to exactly one shard, so two
// writes to different series can proceed in parallel as long as they map to
// different shards. 16 keeps the per-shard mutex cache footprint small while
// giving DogStatsD's worker goroutines enough room to write concurrently.
const numStorageShards = 16

// storageShard owns one slice of the (namespace, name, tags) keyspace. The
// shard mutex protects both the series map and the column data of every
// seriesStats it owns. Cross-shard reads are coordinated by RLock-ing each
// shard independently — the result is per-shard consistent but not
// strictly cross-shard atomic, which is acceptable for analyzer scans.
type storageShard struct {
	mu     sync.RWMutex
	series map[string]*seriesStats
}

// timeSeriesStorage is the engine's metric store. Writes shard by series
// identity; reads either dispatch to the owning shard (by ref or by key) or
// scan all shards (for cross-shard listings).
type timeSeriesStorage struct {
	shards [numStorageShards]storageShard

	// stateMu protects rarely-contended global state: observation
	// timestamps that are not tied to a specific series, and per-metric
	// drop accounting.
	stateMu               sync.Mutex
	observationTimestamps map[int64]struct{}
	droppedNonFinite      int64
	droppedExtreme        int64
	droppedByMetric       map[string]int64
	sampledDrops          map[string]int

	// idMu protects the global compact-ID tables. Append-only writes (new
	// series creation, the cold path) take Lock(); ref-resolution reads
	// (resolveByID, CompactSeriesID, BulkSeriesStatus's ref-grouping) take
	// RLock so detector read paths don't serialize through one mutex.
	idMu          sync.RWMutex
	seriesIDs     map[string]observer.SeriesRef // string key → numeric ref
	seriesIDKeys  []string                      // numeric ID → string key
	seriesIDStats []*seriesStats                // numeric ID → *seriesStats

	// seriesGen increments once per new series. Atomic so SeriesGeneration
	// can read without taking a lock.
	seriesGen atomic.Uint64
}

// seriesStats contains accumulated statistics for a time series (internal).
// Data is stored in columnar layout: parallel arrays indexed by point position.
// Timestamps are stored in sorted order, enabling binary search for range
// queries.
//
// Lifetime: a seriesStats lives in exactly one shard for its entire
// existence. Namespace, Name, Tags, internalKey, and shardIdx are
// effectively immutable after the constructor returns. Column arrays and
// writeGeneration are mutated under the owning shard's write lock.
type seriesStats struct {
	Namespace   string
	Name        string
	Tags        []string
	internalKey string // cached map key to avoid recomputation
	shardIdx    uint8  // index into timeSeriesStorage.shards

	// writeGeneration is per-series and increments on every Add, including
	// same-bucket merges into an existing point.
	writeGeneration int64

	// Columnar storage — all slices have the same length, indexed by point
	// position.
	timestamps []int64
	sums       []float64
	counts     []int64
	mins       []float64
	maxes      []float64
}

func (s *seriesStats) pointCount() int {
	return len(s.timestamps)
}

func (s *seriesStats) sampleCount() int64 {
	count := int64(0)
	for _, c := range s.counts {
		count += c
	}
	return count
}

// Aggregate is an alias to the definition in the observer component for internal use.
type Aggregate = observer.Aggregate

const (
	AggregateAverage = observer.AggregateAverage
	AggregateSum     = observer.AggregateSum
	AggregateCount   = observer.AggregateCount
	AggregateMin     = observer.AggregateMin
	AggregateMax     = observer.AggregateMax
)

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

// newTimeSeriesStorage creates a new time series storage.
func newTimeSeriesStorage() *timeSeriesStorage {
	s := &timeSeriesStorage{
		observationTimestamps: make(map[int64]struct{}),
		seriesIDs:             make(map[string]observer.SeriesRef),
		droppedByMetric:       make(map[string]int64),
		sampledDrops:          make(map[string]int),
	}
	for i := range s.shards {
		s.shards[i].series = make(map[string]*seriesStats)
	}
	return s
}

// shardFor returns the shard that owns the given series identity.
func (s *timeSeriesStorage) shardFor(namespace, name string, tags []string) *storageShard {
	h := hashSeriesIdentity(namespace, name, tags)
	return &s.shards[h%numStorageShards]
}

// Add records a data point for a named metric in a namespace.
// Invalid values are dropped at ingest with accounting and sampled logging.
// Timestamps are maintained in sorted order so replay and live ingestion remain
// correct even when data arrives out of order.
// Returns true if this point created a new series (cardinality +1), false otherwise.
func (s *timeSeriesStorage) Add(namespace, name string, value float64, timestamp int64, tags []string) bool {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		s.recordDroppedValue("non_finite", namespace, name, value, timestamp, tags)
		return false
	}
	// Guard against known finite sentinel values (MaxFloat64 used as "unlimited")
	// that overflow downstream aggregation math when summed.
	if value == math.MaxFloat64 || value == -math.MaxFloat64 {
		s.recordDroppedValue("extreme", namespace, name, value, timestamp, tags)
		return false
	}

	h := hashSeriesIdentity(namespace, name, tags)
	shardIdx := uint8(h % numStorageShards)
	shard := &s.shards[shardIdx]
	key := seriesKey(namespace, name, tags)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	stats, exists := shard.series[key]
	if !exists {
		stats = &seriesStats{
			Namespace:   namespace,
			Name:        name,
			Tags:        canonicalizeTags(tags),
			internalKey: key,
			shardIdx:    shardIdx,
		}
		shard.series[key] = stats
		s.assignID(key, stats)
		s.seriesGen.Add(1)
	}
	isNew := !exists
	stats.writeGeneration++

	// Bucket by second.
	bucket := timestamp

	idx := sort.Search(len(stats.timestamps), func(i int) bool {
		return stats.timestamps[i] >= bucket
	})

	if idx < len(stats.timestamps) && stats.timestamps[idx] == bucket {
		stats.sums[idx] += value
		stats.counts[idx]++
		if value < stats.mins[idx] {
			stats.mins[idx] = value
		}
		if value > stats.maxes[idx] {
			stats.maxes[idx] = value
		}
		return isNew
	}

	stats.timestamps = insertInt64(stats.timestamps, idx, bucket)
	stats.sums = insertFloat64(stats.sums, idx, value)
	stats.counts = insertInt64(stats.counts, idx, 1)
	stats.mins = insertFloat64(stats.mins, idx, value)
	stats.maxes = insertFloat64(stats.maxes, idx, value)
	return isNew
}

// assignID registers a new series with the global compact-ID tables. Caller
// must already hold the owning shard's write lock; idMu is taken briefly
// here. Series creation is rare enough that this serialization is invisible
// in practice.
func (s *timeSeriesStorage) assignID(key string, stats *seriesStats) {
	s.idMu.Lock()
	defer s.idMu.Unlock()
	id := observer.SeriesRef(len(s.seriesIDKeys))
	s.seriesIDs[key] = id
	s.seriesIDKeys = append(s.seriesIDKeys, key)
	s.seriesIDStats = append(s.seriesIDStats, stats)
}

func insertInt64(s []int64, idx int, v int64) []int64 {
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
}

func insertFloat64(s []float64, idx int, v float64) []float64 {
	s = append(s, 0)
	copy(s[idx+1:], s[idx:])
	s[idx] = v
	return s
}

func (s *timeSeriesStorage) recordDroppedValue(reason, namespace, name string, value float64, timestamp int64, tags []string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
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
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

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
		shard := s.shardFor(namespace, name, tags)
		key := seriesKey(namespace, name, tags)
		shard.mu.RLock()
		stats := shard.series[key]
		if stats == nil {
			shard.mu.RUnlock()
			return nil
		}
		series := stats.toSeries(agg)
		shard.mu.RUnlock()
		return &series
	}

	// tags is nil: find first matching series by namespace and name across
	// all shards. The set of (namespace, name, *) hashes to many shards
	// because tags participate in the hash, so we have to scan.
	prefix := namespace + "|" + name + "|"
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for key, stats := range shard.series {
			if strings.HasPrefix(key, prefix) {
				series := stats.toSeries(agg)
				shard.mu.RUnlock()
				return &series
			}
		}
		shard.mu.RUnlock()
	}
	return nil
}

// GetSeriesSince returns points with timestamp > since (for delta updates).
// If since is 0, returns all points.
func (s *timeSeriesStorage) GetSeriesSince(namespace, name string, tags []string, agg Aggregate, since int64) *observer.Series {
	shard := s.shardFor(namespace, name, tags)
	key := seriesKey(namespace, name, tags)

	shard.mu.RLock()
	defer shard.mu.RUnlock()

	stats := shard.series[key]
	if stats == nil {
		return nil
	}

	if since == 0 {
		series := stats.toSeries(agg)
		return &series
	}

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
	seen := make(map[string]struct{})
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			seen[stats.Namespace] = struct{}{}
		}
		shard.mu.RUnlock()
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
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			if stats.Namespace == namespace {
				result = append(result, stats.toSeries(agg))
			}
		}
		shard.mu.RUnlock()
	}
	return result
}

// TimeBounds returns the minimum and maximum timestamps across all stored points.
func (s *timeSeriesStorage) TimeBounds() (minTs int64, maxTs int64, ok bool) {
	var min int64
	var max int64
	found := false

	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			n := stats.pointCount()
			if n == 0 {
				continue
			}
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
		shard.mu.RUnlock()
	}

	return min, max, found
}

// MaxTimestamp returns the latest timestamp across all series in storage.
func (s *timeSeriesStorage) MaxTimestamp() int64 {
	var max int64
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			if n := stats.pointCount(); n > 0 {
				if t := stats.timestamps[n-1]; t > max {
					max = t
				}
			}
		}
		shard.mu.RUnlock()
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

// FNV-1a 64-bit constants, inlined to avoid sync.Pool / hash.Hash allocation.
const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

func fnvUpdateByte(h uint64, b byte) uint64 {
	return (h ^ uint64(b)) * fnvPrime64
}

func fnvUpdateString(h uint64, s string) uint64 {
	for i := 0; i < len(s); i++ {
		h = (h ^ uint64(s[i])) * fnvPrime64
	}
	return h
}

// hashSeriesIdentity hashes (namespace, name, canonicalized tags) into a
// 64-bit identity that matches across equivalent tag orderings. Allocates
// nothing for the common case (≤16 tags), so it is cheap to call on every
// storage write to dispatch to a shard.
func hashSeriesIdentity(namespace, name string, tags []string) uint64 {
	h := uint64(fnvOffset64)
	h = fnvUpdateString(h, namespace)
	h = fnvUpdateByte(h, '|')
	h = fnvUpdateString(h, name)
	h = fnvUpdateByte(h, '|')

	n := len(tags)
	switch n {
	case 0:
		return h
	case 1:
		return fnvUpdateString(h, tags[0])
	}

	if tagsSorted(tags) {
		for i, t := range tags {
			if i > 0 {
				h = fnvUpdateByte(h, ',')
			}
			h = fnvUpdateString(h, t)
		}
		return h
	}

	// Sort an index buffer rather than the caller's slice; for typical tag
	// counts the buffer is stack-allocated.
	var idxBuf [16]int
	var idx []int
	if n <= len(idxBuf) {
		idx = idxBuf[:n]
	} else {
		idx = make([]int, n)
	}
	for i := range idx {
		idx[i] = i
	}
	for i := 1; i < n; i++ {
		for j := i; j > 0 && tags[idx[j-1]] > tags[idx[j]]; j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	for i, k := range idx {
		if i > 0 {
			h = fnvUpdateByte(h, ',')
		}
		h = fnvUpdateString(h, tags[k])
	}
	return h
}

// resolveByID returns the seriesStats for a numeric series ID. Held under
// idMu.RLock so detector reads don't serialize against each other; the
// append-only slice can be safely read concurrently while writers
// (assignID) take Lock(). Callers that read the stats' column data must
// then RLock the owning shard.
func (s *timeSeriesStorage) resolveByID(ref observer.SeriesRef) *seriesStats {
	s.idMu.RLock()
	defer s.idMu.RUnlock()
	if ref < 0 || int(ref) >= len(s.seriesIDStats) {
		return nil
	}
	return s.seriesIDStats[ref]
}

// shardOf returns the shard that owns the given seriesStats.
func (s *timeSeriesStorage) shardOf(stats *seriesStats) *storageShard {
	return &s.shards[stats.shardIdx]
}

// GetSeriesMeta returns the metadata for a series by its numeric ref.
// Returns nil if the ref is out of range.
func (s *timeSeriesStorage) GetSeriesMeta(ref observer.SeriesRef) *observer.SeriesMeta {
	stats := s.resolveByID(ref)
	if stats == nil {
		return nil
	}
	// Namespace, Name, Tags are immutable post-construction; no lock needed.
	return &observer.SeriesMeta{
		Ref:       ref,
		Namespace: stats.Namespace,
		Name:      stats.Name,
		Tags:      stats.Tags,
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
	type entry struct {
		key   string
		stats *seriesStats
		count int
	}
	var entries []entry
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for key, stats := range shard.series {
			if stats.Namespace == namespace {
				entries = append(entries, entry{key: key, stats: stats, count: stats.pointCount()})
			}
		}
		shard.mu.RUnlock()
	}

	if len(entries) == 0 {
		return nil
	}

	// Resolve refs from the global ID table.
	s.idMu.RLock()
	result := make([]seriesMeta, len(entries))
	for i, e := range entries {
		result[i] = seriesMeta{
			Ref:        s.seriesIDs[e.key],
			Namespace:  e.stats.Namespace,
			Name:       e.stats.Name,
			Tags:       copyTags(e.stats.Tags),
			PointCount: e.count,
		}
	}
	s.idMu.RUnlock()

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
	stats := s.resolveByID(ref)
	if stats == nil {
		return nil
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	series := stats.toSeries(agg)
	return &series
}

// ListAllSeriesCompact returns lightweight metadata for every stored series.
func (s *timeSeriesStorage) ListAllSeriesCompact() []seriesCompact {
	var result []seriesCompact
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, st := range shard.series {
			result = append(result, seriesCompact{
				Namespace: st.Namespace,
				Name:      st.Name,
				Tags:      st.Tags,
			})
		}
		shard.mu.RUnlock()
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
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, st := range shard.series {
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
		shard.mu.RUnlock()
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// DataTimestamps returns all unique timestamps that have data, sorted ascending.
func (s *timeSeriesStorage) DataTimestamps() []int64 {
	seen := make(map[int64]struct{})
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			for _, ts := range stats.timestamps {
				seen[ts] = struct{}{}
			}
		}
		shard.mu.RUnlock()
	}
	// Include observation timestamps (e.g., from logs that produced no virtual metrics).
	s.stateMu.Lock()
	for ts := range s.observationTimestamps {
		seen[ts] = struct{}{}
	}
	s.stateMu.Unlock()

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
	return s.seriesGen.Load()
}

// CompactSeriesID translates a full series key to its compact numeric ID string.
// The full key format is "namespace|name:agg|tags" where the storage key is
// "namespace|name|tags" (without the agg suffix).
func (s *timeSeriesStorage) CompactSeriesID(fullKey string) string {
	namespace, nameWithAgg, tags, ok := parseSeriesKey(fullKey)
	if !ok {
		return fullKey
	}

	baseName := nameWithAgg
	aggStr := ""
	if idx := strings.LastIndex(nameWithAgg, ":"); idx > 0 {
		baseName = nameWithAgg[:idx]
		aggStr = nameWithAgg[idx+1:]
	}

	storageKey := seriesKey(namespace, baseName, tags)

	s.idMu.RLock()
	numID, found := s.seriesIDs[storageKey]
	s.idMu.RUnlock()
	if !found {
		return fullKey
	}

	if aggStr != "" {
		return fmt.Sprintf("%d:%s", numID, aggStr)
	}
	return strconv.Itoa(int(numID))
}

// StorageReader interface implementation

// ListSeries returns metadata for all series matching the filter.
func (s *timeSeriesStorage) ListSeries(filter observer.SeriesFilter) []observer.SeriesMeta {
	type entry struct {
		key   string
		stats *seriesStats
	}
	var entries []entry
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
	listSeriesLoop:
		for key, stats := range shard.series {
			if filter.Namespace != "" {
				if stats.Namespace != filter.Namespace {
					continue
				}
			} else {
				for _, ex := range filter.ExcludeNamespaces {
					if stats.Namespace == ex {
						continue listSeriesLoop
					}
				}
			}
			if filter.NamePattern != "" && !strings.HasPrefix(stats.Name, filter.NamePattern) {
				continue
			}
			if !matchTags(stats.Tags, filter.TagMatchers) {
				continue
			}
			entries = append(entries, entry{key: key, stats: stats})
		}
		shard.mu.RUnlock()
	}

	if len(entries) == 0 {
		return nil
	}

	result := make([]observer.SeriesMeta, len(entries))
	s.idMu.RLock()
	for i, e := range entries {
		result[i] = observer.SeriesMeta{
			Ref:       s.seriesIDs[e.key],
			Namespace: e.stats.Namespace,
			Name:      e.stats.Name,
			Tags:      e.stats.Tags,
		}
	}
	s.idMu.RUnlock()
	return result
}

// PointCount returns the number of raw data points for a series.
func (s *timeSeriesStorage) PointCount(ref observer.SeriesRef) int {
	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return stats.pointCount()
}

// TotalSampleCount returns the total number of stored samples across all series,
// excluding series in excludeNamespace (pass "" to include all namespaces).
func (s *timeSeriesStorage) TotalSampleCount(excludeNamespace string) int64 {
	total := int64(0)
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			if excludeNamespace != "" && stats.Namespace == excludeNamespace {
				continue
			}
			total += stats.sampleCount()
		}
		shard.mu.RUnlock()
	}
	return total
}

// TotalSeriesCount returns the number of unique series (name + tag combinations),
// excluding series in excludeNamespace (pass "" to include all namespaces).
func (s *timeSeriesStorage) TotalSeriesCount(excludeNamespace string) int {
	total := 0
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for _, stats := range shard.series {
			if excludeNamespace != "" && stats.Namespace == excludeNamespace {
				continue
			}
			total++
		}
		shard.mu.RUnlock()
	}
	return total
}

// PointCountUpTo returns the number of raw data points with timestamp <= endTime.
// Uses binary search since timestamps are sorted.
func (s *timeSeriesStorage) PointCountUpTo(ref observer.SeriesRef, endTime int64) int {
	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	if stats.pointCount() == 0 {
		return 0
	}
	return searchAfter(stats.timestamps, endTime)
}

// RecordObservationTime records that an observation occurred at the given timestamp.
// This is used for log observations that may not produce virtual metrics but still
// need to appear in DataTimestamps for replay fidelity.
func (s *timeSeriesStorage) RecordObservationTime(timestamp int64) {
	s.stateMu.Lock()
	s.observationTimestamps[timestamp] = struct{}{}
	s.stateMu.Unlock()
}

// WriteGeneration returns a counter that increments on every Add call
// (including same-bucket merges). Detectors use this to detect value
// changes that don't create new buckets.
func (s *timeSeriesStorage) WriteGeneration(ref observer.SeriesRef) int64 {
	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return stats.writeGeneration
}

// BulkSeriesStatus returns the point count (up to endTime) and write generation
// for each ref in a single pass. Each ref's owning shard is RLocked
// individually; cross-ref consistency is per-shard.
func (s *timeSeriesStorage) BulkSeriesStatus(refs []observer.SeriesRef, endTime int64) []seriesStatus {
	result := make([]seriesStatus, len(refs))

	// Group refs by shard to take each shard's RLock once.
	var byShard [numStorageShards][]int
	statsByRef := make([]*seriesStats, len(refs))
	s.idMu.RLock()
	for i, ref := range refs {
		if ref < 0 || int(ref) >= len(s.seriesIDStats) {
			continue
		}
		stats := s.seriesIDStats[ref]
		statsByRef[i] = stats
		byShard[stats.shardIdx] = append(byShard[stats.shardIdx], i)
	}
	s.idMu.RUnlock()

	for shardIdx := range byShard {
		idxs := byShard[shardIdx]
		if len(idxs) == 0 {
			continue
		}
		shard := &s.shards[shardIdx]
		shard.mu.RLock()
		for _, i := range idxs {
			stats := statsByRef[i]
			if stats == nil || stats.pointCount() == 0 {
				continue
			}
			result[i] = seriesStatus{
				pointCount:      searchAfter(stats.timestamps, endTime),
				writeGeneration: stats.writeGeneration,
			}
		}
		shard.mu.RUnlock()
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

// GetSeriesRange returns points within a time range (start, end].
// Start is exclusive, end is inclusive. Use start=0 to read from the beginning.
func (s *timeSeriesStorage) GetSeriesRange(ref observer.SeriesRef, start, end int64, agg Aggregate) *observer.Series {
	stats := s.resolveByID(ref)
	if stats == nil {
		return nil
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	lo := searchAfter(stats.timestamps, start)
	hi := searchAfter(stats.timestamps, end)

	resultLen := hi - lo
	points := make([]observer.Point, resultLen)

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
	default:
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
// per-call allocation.
var pointBufPool = sync.Pool{
	New: func() any { return &[]observer.Point{} },
}

// ForEachPoint calls fn for every point in the time range (start, end].
// The Series pointer is valid only for the duration of the callback.
// Returns false if the series was not found.
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

// SumRange returns the aggregate total over the time range (start, end].
func (s *timeSeriesStorage) SumRange(ref observer.SeriesRef, start, end int64, agg Aggregate) float64 {
	stats := s.resolveByID(ref)
	if stats == nil {
		return 0
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

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
func (s *timeSeriesStorage) snapshotRange(
	ref observer.SeriesRef, start, end int64, agg Aggregate,
	buf []observer.Point,
) (observer.Series, []observer.Point, bool) {
	stats := s.resolveByID(ref)
	if stats == nil {
		return observer.Series{}, buf, false
	}
	shard := s.shardOf(stats)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

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
