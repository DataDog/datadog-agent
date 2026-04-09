// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// splitTagKeyOrder is the canonical ordered list of tag dimensions used to split
// log clusters. The order governs how split-tag summaries are rendered in event
// messages. Add new fields here AND in TagGroupByKey / extractTagGroupByKey.
var splitTagKeyOrder = []string{"source", "service", "env", "host"}

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
func (tc *TaggedPatternClusterer) Process(tags []string, message string, unixSec int64) (uint64, *patterns.Cluster, bool) {
	group := extractTagGroupByKey(tags)
	groupHash := tc.registry.Register(group)

	sub, exists := tc.subClusterers[groupHash]
	if !exists {
		sub = tc.newPatternClusterer()
		tc.subClusterers[groupHash] = sub
	}

	cluster, ok := sub.Process(message, unixSec)
	if !ok {
		return 0, nil, false
	}
	return groupHash, cluster, true
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
// previously registered hashes remain resolvable after a reset.
func (tc *TaggedPatternClusterer) Reset() {
	tc.subClusterers = make(map[uint64]*patterns.PatternClusterer)
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
