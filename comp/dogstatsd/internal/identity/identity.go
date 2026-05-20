// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package identity names DogStatsD series descriptors and view keys.
//
// DogStatsD ingestion currently projects the same parsed sample into several
// keys: batch sharding includes host, serverDebug stats historically do not,
// and backend aggregation is resolved later after tagger enrichment. This
// package centralizes those projections so future migrations can share work
// without treating every projection as a separate semantic identity.
package identity

import (
	"os"
	"strconv"
	"strings"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

const debugTagSeparator = " "

func compactIdentityCacheEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES"))
	return err == nil && enabled
}

func compactIdentityCacheSize() int {
	if raw := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_COMPACT_IDENTITIES_SIZE"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	return 4096
}

// Builder owns scratch buffers used to compute DogStatsD identities.
//
// It is not safe for concurrent use. The zero value is valid.
type Builder struct {
	keyGenerator *ckey.KeyGenerator
	metricTags   *tagset.HashingTagsAccumulator
	compact      *compactIdentityCache
}

// NewBuilder creates a reusable identity builder.
func NewBuilder() *Builder {
	return NewBuilderWithScope(1)
}

// NewBuilderWithScope creates a reusable identity builder whose compact
// identity IDs are unique within scope. DogStatsD workers pass their worker ID
// as the scope so parser-local compact IDs can be carried safely to shared
// downstream workers.
func NewBuilderWithScope(scope uint16) *Builder {
	return &Builder{
		keyGenerator: ckey.NewKeyGenerator(),
		metricTags:   tagset.NewHashingTagsAccumulator(),
		compact:      newCompactIdentityCache(scope, compactIdentityCacheSize()),
	}
}

type compactIdentityKey struct {
	name     string
	host     string
	tagsetID uint64
}

type compactIdentityEntry struct {
	id    uint64
	shard ShardIdentity
	state *metrics.DogStatsDCompactIdentityState
}

type compactIdentityCache struct {
	maxSize int
	scope   uint64
	nextID  uint64
	entries map[compactIdentityKey]compactIdentityEntry
	ring    []compactIdentityKey
	next    int
}

func newCompactIdentityCache(scope uint16, maxSize int) *compactIdentityCache {
	if !compactIdentityCacheEnabled() || maxSize <= 0 {
		return nil
	}
	if scope == 0 {
		scope = 1
	}
	return &compactIdentityCache{
		maxSize: maxSize,
		scope:   uint64(scope) << 48,
		entries: make(map[compactIdentityKey]compactIdentityEntry),
		ring:    make([]compactIdentityKey, maxSize),
	}
}

func (c *compactIdentityCache) lookup(sample metrics.MetricSample) (compactIdentityEntry, bool) {
	if c == nil || sample.DogStatsDTagsetID == 0 {
		return compactIdentityEntry{}, false
	}
	entry, ok := c.entries[compactIdentityKey{name: sample.Name, host: sample.Host, tagsetID: sample.DogStatsDTagsetID}]
	return entry, ok
}

func (c *compactIdentityCache) insert(sample metrics.MetricSample, shard ShardIdentity) (compactIdentityEntry, bool) {
	if c == nil || sample.DogStatsDTagsetID == 0 {
		return compactIdentityEntry{}, false
	}
	key := compactIdentityKey{name: sample.Name, host: sample.Host, tagsetID: sample.DogStatsDTagsetID}
	if entry, ok := c.entries[key]; ok {
		return entry, true
	}
	evicted := c.ring[c.next]
	if evicted.tagsetID != 0 {
		delete(c.entries, evicted)
	}
	entry := compactIdentityEntry{id: c.allocateID(), shard: shard, state: &metrics.DogStatsDCompactIdentityState{}}
	c.ring[c.next] = key
	c.next = (c.next + 1) % len(c.ring)
	c.entries[key] = entry
	return entry, true
}

func (c *compactIdentityCache) allocateID() uint64 {
	c.nextID++
	if c.nextID == 0 || c.nextID >= 1<<48 {
		c.nextID = 1
	}
	return c.scope | c.nextID
}

// ClientSeriesIdentity is the parsed client-facing series identity before
// origin/tagger enrichment.
//
// Name and tags are taken from metrics.MetricSample after DogStatsD parsing,
// metadata extraction, and mapper rewrites. Host has already been extracted out
// of client tags, and is intentionally not part of this identity.
//
// Tags borrows the sample's slice; callers must not retain it beyond the sample
// lifetime unless they make their own copy.
type ClientSeriesIdentity struct {
	Name string
	Tags []string
}

// DebugViewKey is the compatibility grouping key currently used by
// serverDebug stats.
//
// It is a view projection over the parsed client series, not a separate
// semantic series identity. The projection intentionally ignores sample host,
// metric type, sample rate, timestamp, origin, and listener metadata to preserve
// existing `agent dogstatsd-stats` behavior.
type DebugViewKey struct {
	Client      ClientSeriesIdentity
	Key         ckey.ContextKey
	DisplayTags string
}

// ShardIdentity is the identity currently used to choose a DogStatsD aggregation
// pipeline before aggregator context resolution.
//
// It includes the parsed metric name, parsed metric tags, and parsed host. It
// intentionally does not include metric type, sample rate, timestamp, origin, or
// listener metadata.
type ShardIdentity struct {
	Client     ClientSeriesIdentity
	Host       string
	ContextKey ckey.ContextKey
}

// EffectiveBackendIdentitySeed is the DogStatsD-side seed for the eventual
// backend aggregation identity.
//
// This is not the final aggregator context key. The final key is still resolved
// later by the aggregator, after tagger/origin enrichment and optional metric
// tag filtering. The seed records which DogStatsD fields are available before
// that boundary.
type EffectiveBackendIdentitySeed struct {
	Name       string
	Host       string
	MetricTags []string
	MetricType metrics.MetricType
	NoIndex    bool
	Source     metrics.MetricSource
	OriginInfo taggertypes.OriginInfo
}

// GetName implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) GetName() string { return s.Name }

// GetHost implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) GetHost() string { return s.Host }

// GetTags implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) GetTags(taggerBuffer, metricBuffer tagset.TagsAccumulator, tagger tagger.Component) {
	metricBuffer.Append(s.MetricTags...)
	tagger.EnrichTags(taggerBuffer, s.OriginInfo)
}

// GetMetricType implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) GetMetricType() metrics.MetricType { return s.MetricType }

// IsNoIndex implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) IsNoIndex() bool { return s.NoIndex }

// GetSource implements metrics.MetricSampleContext.
func (s *EffectiveBackendIdentitySeed) GetSource() metrics.MetricSource { return s.Source }

var _ metrics.MetricSampleContext = (*EffectiveBackendIdentitySeed)(nil)

// LineageIdentity records transport/origin fields that explain where a sample
// came from, but that are not part of current debug or shard identities.
type LineageIdentity struct {
	ListenerID string
	Source     metrics.MetricSource
	OriginInfo taggertypes.OriginInfo
}

// HotPathContext carries the identities used by the DogStatsD worker hot path.
//
// A value of this type can be carried alongside a MetricSample to let debug
// stats and batch sharding share identity work without forcing downstream
// backend/lineage descriptors into every hot-path operation.
type HotPathContext struct {
	Client    ClientSeriesIdentity
	DebugView DebugViewKey
	Shard     ShardIdentity

	// CompactID is an experimental bounded-dictionary identifier for the parsed
	// DogStatsD identity. A value of 0 means no compact identity is available.
	CompactID uint64

	// CompactState lets downstream experimental consumers acknowledge descriptor
	// state for CompactID without feeding back through the parser worker.
	CompactState *metrics.DogStatsDCompactIdentityState
}

// ResolvedSampleContext groups all named identities derivable from a parsed
// DogStatsD sample before aggregator context resolution.
type ResolvedSampleContext struct {
	Client      ClientSeriesIdentity
	DebugView   DebugViewKey
	Shard       ShardIdentity
	BackendSeed EffectiveBackendIdentitySeed
	Lineage     LineageIdentity
}

// SampleIdentities is kept as a descriptive alias for tests and docs that talk
// about the set of identities rather than the hot-path resolved context.
type SampleIdentities = ResolvedSampleContext

// ClientSeries returns the parsed client-facing series identity for sample.
func ClientSeries(sample metrics.MetricSample) ClientSeriesIdentity {
	return ClientSeriesIdentity{
		Name: sample.Name,
		Tags: sample.Tags,
	}
}

// BackendSeed returns the DogStatsD-side seed for the eventual backend identity.
func BackendSeed(sample metrics.MetricSample) EffectiveBackendIdentitySeed {
	return EffectiveBackendIdentitySeed{
		Name:       sample.Name,
		Host:       sample.Host,
		MetricTags: sample.Tags,
		MetricType: sample.Mtype,
		NoIndex:    sample.NoIndex,
		Source:     sample.Source,
		OriginInfo: sample.OriginInfo,
	}
}

// Lineage returns the sample lineage fields that are available in DogStatsD.
func Lineage(sample metrics.MetricSample) LineageIdentity {
	return LineageIdentity{
		ListenerID: sample.ListenerID,
		Source:     sample.Source,
		OriginInfo: sample.OriginInfo,
	}
}

// DebugView returns the current serverDebug view key for sample.
func (b *Builder) DebugView(sample metrics.MetricSample) DebugViewKey {
	b.ensure()
	defer b.metricTags.Reset()

	b.metricTags.Append(sample.Tags...)
	key := b.keyGenerator.Generate(sample.Name, "", b.metricTags)

	return DebugViewKey{
		Client:      ClientSeries(sample),
		Key:         key,
		DisplayTags: strings.Join(b.metricTags.Get(), debugTagSeparator),
	}
}

// Shard returns the current DogStatsD batch shard identity for sample.
func (b *Builder) Shard(sample metrics.MetricSample) ShardIdentity {
	b.ensure()
	defer b.metricTags.Reset()

	b.metricTags.Append(sample.Tags...)
	key := b.keyGenerator.Generate(sample.Name, sample.Host, b.metricTags)

	return ShardIdentity{
		Client:     ClientSeries(sample),
		Host:       sample.Host,
		ContextKey: key,
	}
}

// ResolveShardHotPath returns the shard identity used by the DogStatsD worker
// hot path. When the experimental compact identity dictionary is enabled and
// the parser provided a compact tagset ID, repeated identities can reuse the
// precomputed shard key and carry a compact ID downstream.
func (b *Builder) ResolveShardHotPath(sample metrics.MetricSample) HotPathContext {
	b.ensure()
	if entry, ok := b.compact.lookup(sample); ok {
		return HotPathContext{
			Client:       entry.shard.Client,
			Shard:        entry.shard,
			CompactID:    entry.id,
			CompactState: entry.state,
		}
	}
	shard := b.Shard(sample)
	entry, ok := b.compact.insert(sample, shard)
	if !ok {
		return HotPathContext{Client: shard.Client, Shard: shard}
	}
	return HotPathContext{
		Client:       shard.Client,
		Shard:        shard,
		CompactID:    entry.id,
		CompactState: entry.state,
	}
}

// ResolveHotPath returns the identities used by the DogStatsD worker hot path.
func (b *Builder) ResolveHotPath(sample metrics.MetricSample) HotPathContext {
	b.ensure()
	defer b.metricTags.Reset()

	client := ClientSeries(sample)
	b.metricTags.Append(sample.Tags...)
	debugViewKey := b.keyGenerator.Generate(sample.Name, "", b.metricTags)
	debugView := DebugViewKey{
		Client:      client,
		Key:         debugViewKey,
		DisplayTags: strings.Join(b.metricTags.Get(), debugTagSeparator),
	}
	shardKey := b.keyGenerator.Generate(sample.Name, sample.Host, b.metricTags)

	return HotPathContext{
		Client:    client,
		DebugView: debugView,
		Shard: ShardIdentity{
			Client:     client,
			Host:       sample.Host,
			ContextKey: shardKey,
		},
	}
}

// Resolve returns all identities currently derivable from sample in DogStatsD.
func (b *Builder) Resolve(sample metrics.MetricSample) ResolvedSampleContext {
	hotPath := b.ResolveHotPath(sample)
	return ResolvedSampleContext{
		Client:      hotPath.Client,
		DebugView:   hotPath.DebugView,
		Shard:       hotPath.Shard,
		BackendSeed: BackendSeed(sample),
		Lineage:     Lineage(sample),
	}
}

// ShardIndex applies the current DogStatsD context-key-to-shard mapping.
func ShardIndex(key ckey.ContextKey, shardCount int) uint32 {
	return uint32((uint64(key>>32) * uint64(shardCount)) >> 32)
}

func (b *Builder) ensure() {
	if b.keyGenerator == nil {
		b.keyGenerator = ckey.NewKeyGenerator()
	}
	if b.metricTags == nil {
		b.metricTags = tagset.NewHashingTagsAccumulator()
	}
}
