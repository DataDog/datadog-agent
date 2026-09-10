// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package infraattributesprocessor

import (
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// maxEntitiesPerResource is the number of entities entityIDsFromAttributes can
// derive from a single resource. Used only as a map sizing hint.
const maxEntitiesPerResource = 8

// tagCacheKey identifies one tagger lookup. types.EntityID is a comparable
// struct (a prefix plus an ID string), so this is a valid map key.
type tagCacheKey struct {
	entityID    types.EntityID
	cardinality types.TagCardinality
}

// tagBatch resolves tagger tags for the resources of a single pdata batch,
// querying the tagger at most once per distinct (entity, cardinality) pair and
// at most once for the global tags.
//
// Resources in a batch overwhelmingly share entities -- every span of a pod
// carries the same container ID, pod UID and namespace -- and the global tags
// are by definition the same for all of them. Without memoization ProcessTags
// re-queries the tagger for each of the (up to maxEntitiesPerResource)
// entities of every resource, plus the global tags, and every one of those
// queries allocates a fresh []string.
//
// A tagBatch is created per batch and never shared: it is a local of the
// process<Signal> functions, so it needs no locking and cannot carry state
// from one batch into the next.
//
// The flip side is deliberate: all resources in one batch now observe a single
// tagger snapshot, where previously a tagger update landing mid-batch could be
// picked up by later resources. Consistency within a batch is the more useful
// property, and the delay is bounded by one batch.
type tagBatch struct {
	p infraTagsProcessor

	// entries memoizes tagger.Tag. Left nil until the first lookup, since a
	// batch whose resources carry no recognized entity never touches it.
	entries map[tagCacheKey][]string

	// globals memoizes tagger.GlobalTags. globalsCardinality guards the case
	// of one batch processed at more than one cardinality -- not something the
	// current callers do, but cheap to be correct about.
	globals            []string
	globalsCardinality types.TagCardinality
	globalsCached      bool

	// scratch holds the resolved tag slices of the resource currently being
	// processed. It lives on the batch, and is therefore allocated once for
	// the whole batch rather than once per resource. See resolve.
	scratch [][]string
}

// newTagBatch returns a tagBatch covering one pdata batch. Callers must not
// reuse it across batches.
func (p infraTagsProcessor) newTagBatch() *tagBatch {
	return &tagBatch{p: p}
}

// tagsFor returns the tagger's tags for one entity.
//
// The returned slice is shared by every resource in the batch that resolves
// the same entity, and callers must only read from it. That is safe: the
// taggers hand out slices the caller owns -- the local tagger copies its
// cached HashedTags, the remote tagger builds a fresh slice from the entity's
// tag arrays -- and ProcessTags only reads them.
//
// Failures are memoized too, so a broken entity is reported once per batch
// rather than once per resource.
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

// resolve looks up the tags of every entity of one resource and returns them
// alongside their total count, which ProcessTags uses to size its tag map in a
// single allocation.
//
// The returned slice of slices is scratch space owned by the batch: it is
// valid until the next resolve call, which is all ProcessTags needs.
func (b *tagBatch) resolve(logger *zap.Logger, entityIDs []types.EntityID, cardinality types.TagCardinality) (resolved [][]string, tagCount int) {
	b.scratch = b.scratch[:0]
	for _, entityID := range entityIDs {
		entityTags := b.tagsFor(logger, entityID, cardinality)
		b.scratch = append(b.scratch, entityTags)
		tagCount += len(entityTags)
	}
	return b.scratch, tagCount
}

// globalTags returns the tagger's global tags for the batch. They do not
// depend on the resource, so one query covers every resource in the batch.
func (b *tagBatch) globalTags(logger *zap.Logger, cardinality types.TagCardinality) []string {
	if b.globalsCached && b.globalsCardinality == cardinality {
		return b.globals
	}
	globalTags, err := b.p.tagger.GlobalTags(cardinality)
	if err != nil {
		// Same as the uncached behavior: report the failure and carry on with
		// whatever came back (nil, in every implementation).
		logger.Error("Cannot get global tags", zap.Error(err))
	}
	b.globals, b.globalsCardinality, b.globalsCached = globalTags, cardinality, true
	return globalTags
}
