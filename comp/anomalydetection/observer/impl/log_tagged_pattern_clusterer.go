// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"container/heap"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl/patterns"
)

// TagGroupByKey holds the tags that are responsible for grouping logs into different clusters.
// Absent tags (e.g. a log with no "env" tag) are represented by an empty string.
type TagGroupByKey struct {
	// Warning: Don't forget to update functions parsing tags when adding new fields
	Source  string
	Service string
	Env     string
	Host    string
}

// AsMap returns a map of non-empty tag key→value pairs for this group.
func (c TagGroupByKey) AsMap() map[string]string {
	m := make(map[string]string, 4)
	if c.Source != "" {
		m["source"] = c.Source
	}
	if c.Service != "" {
		m["service"] = c.Service
	}
	if c.Env != "" {
		m["env"] = c.Env
	}
	if c.Host != "" {
		m["host"] = c.Host
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// tagGroupByKeyHash computes a stable fnv64a hash for a TagGroupByKey.
func tagGroupByKeyHash(c TagGroupByKey) uint64 {
	h := fnv.New64a()
	h.Write([]byte(c.Source))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Service))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Env))
	h.Write([]byte{'|'})
	h.Write([]byte(c.Host))
	return h.Sum64()
}

// TagGroupByKeyRegistry is a bidirectional, append-only store between a uint64 hash
// and a TagGroupByKey. It is NOT thread-safe; access must be confined to a single goroutine.
type TagGroupByKeyRegistry struct {
	byHash map[uint64]TagGroupByKey
}

// NewTagGroupByKeyRegistry creates an empty TagGroupByKeyRegistry.
func NewTagGroupByKeyRegistry() *TagGroupByKeyRegistry {
	return &TagGroupByKeyRegistry{byHash: make(map[uint64]TagGroupByKey)}
}

// Register inserts (or confirms) a TagGroupByKey and returns its stable hash.
// Calling Register twice with the same group returns the same hash.
func (r *TagGroupByKeyRegistry) Register(group TagGroupByKey) uint64 {
	hash := tagGroupByKeyHash(group)
	if _, exists := r.byHash[hash]; !exists {
		r.byHash[hash] = group
	}
	return hash
}

// Lookup returns the TagGroupByKey for the given hash, and whether it was found.
func (r *TagGroupByKeyRegistry) Lookup(hash uint64) (TagGroupByKey, bool) {
	group, ok := r.byHash[hash]
	return group, ok
}

// extractTagGroupByKey scans a flat "key:value" tag slice and extracts the
// four fixed split dimensions (source, service, env, host).
// Missing dimensions are left as empty strings.
func extractTagGroupByKey(tags []string) TagGroupByKey {
	var group TagGroupByKey
	for _, tag := range tags {
		idx := strings.IndexByte(tag, ':')
		if idx < 0 {
			continue
		}
		k, v := tag[:idx], tag[idx+1:]
		switch k {
		case "source":
			group.Source = v
		case "service":
			group.Service = v
		case "env":
			group.Env = v
		case "host":
			group.Host = v
		}
	}
	return group
}

// tagsForPatternGrouping returns a tag slice for TaggedPatternClusterer.Process.
// When tags omit `host:` but hostname is non-empty (e.g. from LogView.GetHostname()),
// a synthetic `host:<hostname>` entry is appended so grouping matches the host
// split dimension. An explicit `host:` tag in tags always wins.
func tagsForPatternGrouping(tags []string, hostname string) []string {
	if hostname == "" {
		return tags
	}
	if extractTagGroupByKey(tags).Host != "" {
		return tags
	}
	out := make([]string, len(tags)+1)
	copy(out, tags)
	out[len(tags)] = "host:" + hostname
	return out
}

// globalClusterHash produces a hex string that stably encodes a (groupHash,
// clusterID) pair. It is used as the variable segment of the metric name so
// that each (tag-group × pattern) combination gets a unique, stable name.
func globalClusterHash(groupHash uint64, clusterID int64) string {
	h := fnv.New64a()
	_ = binary.Write(h, binary.LittleEndian, groupHash)
	_ = binary.Write(h, binary.LittleEndian, clusterID)
	return strconv.FormatUint(h.Sum64(), 16)
}

// TaggedPatternClusterer wraps one *patterns.PatternClusterer per tag-group
// hash so that each unique (source, service, env, host) combination is
// clustered independently.
//
// NOT thread-safe: all calls must be made from the same goroutine.
type TaggedPatternClusterer struct {
	registry            *TagGroupByKeyRegistry
	subClusterers       map[uint64]*patterns.PatternClusterer
	newPatternClusterer func() *patterns.PatternClusterer
	// MaxClustersPerGroup, when > 0, is propagated as patterns.PatternClusterer.MaxClusters
	// on each newly created sub-clusterer; existing sub-clusterers are NOT
	// retroactively updated. Zero means unbounded.
	MaxClustersPerGroup int
	// MaxTagGroups, when > 0, caps the number of live sub-clusterers. When a new
	// tag group would push subClusterers past this size, the least-recently-
	// touched group (smallest entry in lastTouchByGroup) is evicted; all of
	// its clusters are surfaced via DrainLRUEvictions. Zero disables the cap.
	MaxTagGroups int
	// lastTouchByGroup tracks the most recent unixSec passed to Process for
	// each group; consulted only when MaxTagGroups > 0.
	lastTouchByGroup map[uint64]int64
	// touchHeap is a lazy-deletion min-heap of (touch, groupHash) entries.
	// Each Process call that updates lastTouchByGroup pushes a new entry rather
	// than performing an in-place decrease-key (which would be O(N) in
	// container/heap). On eviction we pop until the top entry's touch matches
	// the current lastTouchByGroup value for its hash — stale entries (older
	// touch than the current map value, or a hash already deleted from the map)
	// are silently dropped. This replaces an O(N) full scan over subClusterers
	// with amortised O(log N) eviction.
	//
	// Bounded growth: the heap can accumulate at most one entry per touch;
	// when it exceeds heapCompactionThreshold * len(subClusterers) we rebuild
	// it from lastTouchByGroup so memory stays O(MaxTagGroups).
	touchHeap *groupTouchHeap
	// lruEvicted accumulates per-cluster evictions from both layer-1 (per-group
	// MaxClusters cap inside a sub-clusterer) and layer-2 (MaxTagGroups cap
	// here) since the last DrainLRUEvictions.
	lruEvicted []EvictedCluster
}

// NewTaggedPatternClusterer creates a TaggedPatternClusterer that writes group
// hashes into registry.
func NewTaggedPatternClusterer(registry *TagGroupByKeyRegistry) *TaggedPatternClusterer {
	return NewTaggedPatternClustererWithFactory(registry, patterns.NewPatternClusterer)
}

// NewTaggedPatternClustererWithFactory is like NewTaggedPatternClusterer but uses newPC
// to construct each per-tag-group sub-clusterer (e.g. to plug tokenizer hyperparameters).
func NewTaggedPatternClustererWithFactory(registry *TagGroupByKeyRegistry, newPC func() *patterns.PatternClusterer) *TaggedPatternClusterer {
	if newPC == nil {
		newPC = patterns.NewPatternClusterer
	}
	return &TaggedPatternClusterer{
		registry:            registry,
		subClusterers:       make(map[uint64]*patterns.PatternClusterer),
		newPatternClusterer: newPC,
	}
}

// Process extracts the tag group from tags, routes the message to the matching
// sub-clusterer (created lazily), and returns the group hash plus the cluster.
// unixSec is Unix seconds for timestamp tracking (use time.Now().Unix() when unknown).
//
// LRU evictions (if any) caused by this call — from per-group MaxClusters or
// global MaxTagGroups caps — must be retrieved via DrainLRUEvictions before
// the next Process call to avoid silently dropping eviction context.
func (tc *TaggedPatternClusterer) Process(tags []string, message string, unixSec int64) (uint64, *patterns.Cluster, bool) {
	group := extractTagGroupByKey(tags)
	groupHash := tc.registry.Register(group)

	sub, exists := tc.subClusterers[groupHash]
	if !exists {
		// Two-phase create: build a transient sub-clusterer but do NOT
		// commit it (insert into tc.subClusterers, possibly evicting the
		// LRU group to make room) until sub.Process actually accepts the
		// message. Otherwise an empty/whitespace-only first message from
		// a new tag group — which PatternClusterer rejects when IgnoreEmpty
		// is on — would steal a slot from an active group while leaving an
		// empty sub-clusterer behind to count against MaxTagGroups. A burst
		// of empty logs from new containers could then evict real pattern
		// state and suppress later anomalies.
		sub = tc.newPatternClusterer()
		sub.MaxClusters = tc.MaxClustersPerGroup
	}

	cluster, ok := sub.Process(message, unixSec)
	if !ok {
		// Transient sub-clusterer (when !exists) is dropped on the floor
		// here — nothing was ever inserted into tc.subClusterers, so no
		// eviction or LRU bookkeeping is needed.
		return 0, nil, false
	}

	// Process accepted the message; only now do we commit the new
	// sub-clusterer (and evict the LRU group if we've hit the cap).
	if !exists {
		tc.evictLRUTagGroupIfOverCap(groupHash)
		tc.subClusterers[groupHash] = sub
	}

	// Drain layer-1 LRU evictions from this sub-clusterer and tag them with groupHash.
	if evicted := sub.DrainLRUEvictedClusterIDs(); len(evicted) > 0 {
		for _, id := range evicted {
			tc.lruEvicted = append(tc.lruEvicted, EvictedCluster{GroupHash: groupHash, ClusterID: id})
		}
	}

	if tc.MaxTagGroups > 0 {
		if tc.lastTouchByGroup == nil {
			tc.lastTouchByGroup = make(map[uint64]int64)
		}
		tc.lastTouchByGroup[groupHash] = unixSec
		if tc.touchHeap == nil {
			tc.touchHeap = &groupTouchHeap{}
			heap.Init(tc.touchHeap)
		}
		heap.Push(tc.touchHeap, groupTouchEntry{touch: unixSec, hash: groupHash})
		tc.maybeCompactTouchHeap()
	}

	return groupHash, cluster, true
}

// evictLRUTagGroupIfOverCap removes the least-recently-touched tag group when
// adding a new group would exceed MaxTagGroups. The about-to-be-added groupHash
// is excluded from eviction. All clusters belonging to the evicted group are
// surfaced via DrainLRUEvictions and the group is removed from
// lastTouchByGroup. The group's hash remains in the registry (registry is
// append-only by design).
//
// Implementation: pops stale entries off touchHeap (entries whose touch no
// longer matches lastTouchByGroup, or whose hash has already been deleted)
// until the top is a valid victim, then evicts it. Groups never touched by
// Process (i.e. absent from lastTouchByGroup) cannot be victims because their
// hash never appears in the heap; in that pathological case we fall back to
// the original O(N) scan so behaviour matches the pre-heap implementation.
func (tc *TaggedPatternClusterer) evictLRUTagGroupIfOverCap(incoming uint64) {
	if tc.MaxTagGroups <= 0 || len(tc.subClusterers) < tc.MaxTagGroups {
		return
	}
	if tc.touchHeap != nil {
		for tc.touchHeap.Len() > 0 {
			top := (*tc.touchHeap)[0]
			if top.hash == incoming {
				// Skip the incoming group: it would be re-pushed and we don't
				// want to evict it. Pop the stale entry and try the next.
				heap.Pop(tc.touchHeap)
				continue
			}
			current, present := tc.lastTouchByGroup[top.hash]
			if !present || current != top.touch {
				// Stale entry: the hash was either evicted earlier or has a
				// newer touch already in the heap. Drop it and continue.
				heap.Pop(tc.touchHeap)
				continue
			}
			// Valid victim.
			heap.Pop(tc.touchHeap)
			if sub, ok := tc.subClusterers[top.hash]; ok {
				for _, c := range sub.GetClusters() {
					tc.lruEvicted = append(tc.lruEvicted, EvictedCluster{GroupHash: top.hash, ClusterID: c.ID})
				}
			}
			delete(tc.subClusterers, top.hash)
			delete(tc.lastTouchByGroup, top.hash)
			return
		}
	}
	// Fallback: heap was empty (e.g. groups never touched while MaxTagGroups
	// was 0) but we are now over cap. Use the original O(N) scan to find a
	// victim. This path is exercised only when MaxTagGroups is enabled
	// retroactively after Process was already called with it disabled.
	var victim uint64
	var victimUnix int64
	victimSet := false
	for gh := range tc.subClusterers {
		if gh == incoming {
			continue
		}
		touch := tc.lastTouchByGroup[gh]
		if !victimSet || touch < victimUnix {
			victim = gh
			victimUnix = touch
			victimSet = true
		}
	}
	if !victimSet {
		return
	}
	if sub, ok := tc.subClusterers[victim]; ok {
		for _, c := range sub.GetClusters() {
			tc.lruEvicted = append(tc.lruEvicted, EvictedCluster{GroupHash: victim, ClusterID: c.ID})
		}
	}
	delete(tc.subClusterers, victim)
	delete(tc.lastTouchByGroup, victim)
}

// heapCompactionThreshold sets when maybeCompactTouchHeap rebuilds touchHeap
// from scratch: when the heap has more than threshold * len(subClusterers)
// entries the cost of rebuilding is amortised against the savings from no
// longer carrying stale entries through future heap operations. Tuned so
// rebuilds happen rarely under steady-state churn but reliably under heavy
// per-group re-touching.
const heapCompactionThreshold = 4

func (tc *TaggedPatternClusterer) maybeCompactTouchHeap() {
	if tc.touchHeap == nil {
		return
	}
	if tc.touchHeap.Len() <= heapCompactionThreshold*len(tc.lastTouchByGroup) {
		return
	}
	rebuilt := make(groupTouchHeap, 0, len(tc.lastTouchByGroup))
	for hash, touch := range tc.lastTouchByGroup {
		rebuilt = append(rebuilt, groupTouchEntry{touch: touch, hash: hash})
	}
	heap.Init(&rebuilt)
	tc.touchHeap = &rebuilt
}

// groupTouchEntry is one entry in the lazy-deletion min-heap used by
// evictLRUTagGroupIfOverCap. Entries become stale when the matching hash
// receives a newer touch (a fresh entry is pushed; the old one is left to be
// skipped at the top of the heap) or when the hash is evicted entirely
// (no longer present in lastTouchByGroup).
type groupTouchEntry struct {
	touch int64
	hash  uint64
}

// groupTouchHeap is a min-heap of groupTouchEntry ordered by touch ascending.
// Implements heap.Interface.
type groupTouchHeap []groupTouchEntry

func (h groupTouchHeap) Len() int           { return len(h) }
func (h groupTouchHeap) Less(i, j int) bool { return h[i].touch < h[j].touch }
func (h groupTouchHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *groupTouchHeap) Push(x any)        { *h = append(*h, x.(groupTouchEntry)) }
func (h *groupTouchHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// DrainLRUEvictions returns and clears all LRU evictions accumulated since the
// last call. Includes both per-group MaxClusters evictions and whole-group
// MaxTagGroups evictions. GC evictions go through GarbageCollectBefore instead.
func (tc *TaggedPatternClusterer) DrainLRUEvictions() []EvictedCluster {
	if len(tc.lruEvicted) == 0 {
		return nil
	}
	out := tc.lruEvicted
	tc.lruEvicted = nil
	return out
}

// GetCluster retrieves a cluster by group hash and intra-clusterer ID.
func (tc *TaggedPatternClusterer) GetCluster(groupHash uint64, clusterID int64) (*patterns.Cluster, error) {
	sub, ok := tc.subClusterers[groupHash]
	if !ok {
		return nil, fmt.Errorf("no sub-clusterer for group hash %x", groupHash)
	}
	return sub.GetCluster(clusterID)
}

// Reset drops all sub-clusterers. The registry is intentionally kept so that
// previously registered hashes remain resolvable after a reset. LRU bookkeeping
// (lastTouchByGroup, pending evictions) is also cleared.
func (tc *TaggedPatternClusterer) Reset() {
	tc.subClusterers = make(map[uint64]*patterns.PatternClusterer)
	tc.lastTouchByGroup = nil
	tc.touchHeap = nil
	tc.lruEvicted = nil
}

// NumSubClusterers returns the number of currently active sub-clusterers.
func (tc *TaggedPatternClusterer) NumSubClusterers() int {
	return len(tc.subClusterers)
}

// EvictedCluster identifies a cluster that was removed during garbage collection,
// pairing its tag-group hash with its intra-clusterer ID.
type EvictedCluster struct {
	GroupHash uint64
	ClusterID int64
}

// GarbageCollectBefore removes all clusters whose LastSeenUnix is strictly less
// than cutoff from every sub-clusterer and returns the (GroupHash, ClusterID)
// pairs that were removed.
func (tc *TaggedPatternClusterer) GarbageCollectBefore(cutoff int64) []EvictedCluster {
	var evicted []EvictedCluster
	for groupHash, sub := range tc.subClusterers {
		stale := sub.ClusterIDsBeforeUnix(cutoff)
		for _, id := range stale {
			evicted = append(evicted, EvictedCluster{GroupHash: groupHash, ClusterID: id})
		}
		if len(stale) > 0 {
			_ = sub.RemoveClusters(stale)
		}
	}
	return evicted
}

// TaggedClusterEntry pairs a cluster with its tag-group hash so callers can
// compute the correct globalClusterHash.
type TaggedClusterEntry struct {
	GroupHash uint64
	Cluster   *patterns.Cluster
}

// GetAllClusters returns every cluster across all sub-clusterers, each paired
// with its group hash.
func (tc *TaggedPatternClusterer) GetAllClusters() []TaggedClusterEntry {
	var entries []TaggedClusterEntry
	for groupHash, sub := range tc.subClusterers {
		for _, c := range sub.GetClusters() {
			entries = append(entries, TaggedClusterEntry{GroupHash: groupHash, Cluster: c})
		}
	}
	return entries
}

// Classify returns the cluster that best matches message within the
// sub-clusterer identified by groupHash, or nil if no match is found.
func (tc *TaggedPatternClusterer) Classify(groupHash uint64, message string) *patterns.Cluster {
	sub, ok := tc.subClusterers[groupHash]
	if !ok {
		return nil
	}
	return sub.Classify(message)
}
