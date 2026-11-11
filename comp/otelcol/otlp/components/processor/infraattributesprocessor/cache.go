// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// maxEntitiesPerResource is how many entities entityIDsFromAttributes can derive from one
// resource. Only used to size slices, so drifting from that function costs nothing.
const maxEntitiesPerResource = 8

// tagCacheKey identifies one tagger lookup.
type tagCacheKey struct {
	entityID    types.EntityID
	cardinality types.TagCardinality
}

// tagBatch memoizes tagger lookups across the resources of one pdata batch, which
// overwhelmingly share entities (every span of a pod carries the same container ID, pod UID
// and namespace). Each lookup otherwise allocates a fresh []string per resource.
//
// It is a local of the process<Signal> functions, so it needs no locking and carries no
// state between batches. The trade-off is deliberate: all resources in a batch now see one
// tagger snapshot, delaying a mid-batch tagger update by at most one batch.
type tagBatch struct {
	p infraTagsProcessor

	// entries memoizes tagger.Tag. Nil until the first lookup.
	entries map[tagCacheKey][]string

	// globals memoizes tagger.GlobalTags; globalsCardinality guards one batch processed
	// at more than one cardinality.
	globals            []string
	globalsCardinality types.TagCardinality
	globalsCached      bool

	// scratch holds the current resource's resolved tag slices, allocated once per batch
	// rather than once per resource. See resolve.
	scratch [][]string
}

// newTagBatch returns a tagBatch for one pdata batch. Do not reuse across batches.
func (p infraTagsProcessor) newTagBatch() *tagBatch {
	return &tagBatch{p: p}
}

// tagsFor returns the tagger's tags for one entity. The slice is shared by every resource
// in the batch resolving the same entity and must only be read; both taggers hand out
// slices the caller owns, and ProcessTags only reads them. Failures are memoized too, so a
// broken entity is logged once per batch.
func (b *tagBatch) tagsFor(logger *zap.Logger, entityID types.EntityID, cardinality types.TagCardinality) []string {
	key := tagCacheKey{entityID: entityID, cardinality: cardinality}
	if entityTags, ok := b.entries[key]; ok { // reading a nil map is fine
		return entityTags
	}
	entityTags, err := b.p.tagger.Tag(entityID, cardinality)
	if err != nil {
		logger.Error("Cannot get tags for entity", zap.String("entityID", entityID.String()), zap.Error(err))
		entityTags = nil
	}
	if b.entries == nil {
		b.entries = make(map[tagCacheKey][]string, maxEntitiesPerResource)
	}
	b.entries[key] = entityTags
	return entityTags
}

// resolve returns the tags of every entity of one resource plus their total count, which
// ProcessTags uses to size its tag map in one allocation. The result is batch-owned scratch
// space, valid until the next resolve call.
func (b *tagBatch) resolve(logger *zap.Logger, entityIDs []types.EntityID, cardinality types.TagCardinality) (resolved [][]string, tagCount int) {
	b.scratch = b.scratch[:0]
	for _, entityID := range entityIDs {
		entityTags := b.tagsFor(logger, entityID, cardinality)
		b.scratch = append(b.scratch, entityTags)
		tagCount += len(entityTags)
	}
	return b.scratch, tagCount
}

// globalTags returns the batch's global tags: resource-independent, so one query covers all.
func (b *tagBatch) globalTags(logger *zap.Logger, cardinality types.TagCardinality) []string {
	if b.globalsCached && b.globalsCardinality == cardinality {
		return b.globals
	}
	globalTags, err := b.p.tagger.GlobalTags(cardinality)
	if err != nil {
		// As before: report and carry on with whatever came back.
		logger.Error("Cannot get global tags", zap.Error(err))
	}
	b.globals, b.globalsCardinality, b.globalsCached = globalTags, cardinality, true
	return globalTags
}
