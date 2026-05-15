// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package identity names the currently distinct DogStatsD metric identities.
//
// DogStatsD ingestion intentionally has more than one identity today. The debug
// stats map, batch sharding, and backend aggregation do not key on exactly the
// same fields. This package gives those contracts explicit names so future
// migrations can share work without accidentally changing semantics.
package identity

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

const debugTagSeparator = " "

// Builder owns scratch buffers used to compute DogStatsD identities.
//
// It is not safe for concurrent use. The zero value is valid.
type Builder struct {
	keyGenerator *ckey.KeyGenerator
	metricTags   *tagset.HashingTagsAccumulator
}

// NewBuilder creates a reusable identity builder.
func NewBuilder() *Builder {
	return &Builder{
		keyGenerator: ckey.NewKeyGenerator(),
		metricTags:   tagset.NewHashingTagsAccumulator(),
	}
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

// DebugIdentity is the identity currently used by serverDebug stats.
//
// It is equivalent to hashing the ClientSeriesIdentity with an empty host. This
// intentionally ignores sample host, metric type, sample rate, timestamp,
// origin, and listener metadata.
type DebugIdentity struct {
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

// LineageIdentity records transport/origin fields that explain where a sample
// came from, but that are not part of current debug or shard identities.
type LineageIdentity struct {
	ListenerID string
	Source     metrics.MetricSource
	OriginInfo taggertypes.OriginInfo
}

// SampleIdentities groups the named identities derivable from a parsed
// DogStatsD sample before aggregator context resolution.
type SampleIdentities struct {
	Client      ClientSeriesIdentity
	Debug       DebugIdentity
	Shard       ShardIdentity
	BackendSeed EffectiveBackendIdentitySeed
	Lineage     LineageIdentity
}

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

// Debug returns the current serverDebug identity for sample.
func (b *Builder) Debug(sample metrics.MetricSample) DebugIdentity {
	b.ensure()
	defer b.metricTags.Reset()

	b.metricTags.Append(sample.Tags...)
	key := b.keyGenerator.Generate(sample.Name, "", b.metricTags)

	return DebugIdentity{
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

// Resolve returns all identities currently derivable from sample in DogStatsD.
func (b *Builder) Resolve(sample metrics.MetricSample) SampleIdentities {
	return SampleIdentities{
		Client:      ClientSeries(sample),
		Debug:       b.Debug(sample),
		Shard:       b.Shard(sample),
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
