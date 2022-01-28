// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// EntityTags holds the tag information for a given entity. It is not
// thread-safe, so should not be shared outside of the store. Usage inside the
// store is safe since it relies on a global lock.
type EntityTags struct {
	entityID              string
	sourceTags            map[string]sourceTags
	cacheValid            bool
	cachedAll             *tagset.Tags // Low + orchestrator + high
	cachedHigh            *tagset.Tags // High only
	cachedOrchestrator    *tagset.Tags // Orchestrator only
	cachedLowOrchestrator *tagset.Tags // Low + Orchestrator
	cachedLow             *tagset.Tags // Low only
}

func newEntityTags(entityID string) *EntityTags {
	return &EntityTags{
		entityID:              entityID,
		sourceTags:            make(map[string]sourceTags),
		cacheValid:            true,
		cachedAll:             tagset.EmptyTags,
		cachedHigh:            tagset.EmptyTags,
		cachedOrchestrator:    tagset.EmptyTags,
		cachedLowOrchestrator: tagset.EmptyTags,
		cachedLow:             tagset.EmptyTags,
	}
}

func (e *EntityTags) getStandard() []string {
	tags := []string{}
	for _, t := range e.sourceTags {
		tags = append(tags, t.standardTags...)
	}
	return tags
}

func (e *EntityTags) get(cardinality collectors.TagCardinality) []string {
	tags := e.getHashedTags(cardinality).UnsafeReadOnlySlice()
	// copy this slice, to avoid aliasing issues (TODO: use *Tags)
	rv := make([]string, len(tags))
	copy(rv, tags)
	return rv
}

func (e *EntityTags) getHashedTags(cardinality collectors.TagCardinality) *tagset.Tags {
	e.computeCache()

	if cardinality == collectors.HighCardinality {
		return e.cachedAll
	} else if cardinality == collectors.OrchestratorCardinality {
		return e.cachedLowOrchestrator
	}
	return e.cachedLow
}

func (e *EntityTags) toEntity() types.Entity {
	e.computeCache()

	return types.Entity{
		ID:                          e.entityID,
		StandardTags:                e.getStandard(),
		HighCardinalityTags:         e.cachedHigh.UnsafeReadOnlySlice(),         // TODO: Entity doesn't use *Tags
		OrchestratorCardinalityTags: e.cachedOrchestrator.UnsafeReadOnlySlice(), // TODO: Entity doesn't use *Tags
		LowCardinalityTags:          e.cachedLow.UnsafeReadOnlySlice(),          // TODO: Entity doesn't use *Tags
	}
}

func (e *EntityTags) computeCache() {
	if e.cacheValid {
		return
	}

	tagList := make(map[collectors.TagCardinality][]string)
	tagMap := make(map[string]collectors.CollectorPriority)

	var sources []string
	for source := range e.sourceTags {
		sources = append(sources, source)
	}

	// sort sources in descending order of priority. assumes lowest if
	// priority is not declared
	sort.Slice(sources, func(i, j int) bool {
		sourceI := sources[i]
		sourceJ := sources[j]
		return collectors.CollectorPriorities[sourceI] > collectors.CollectorPriorities[sourceJ]
	})

	// insertWithPriority prevents two collectors of different priorities
	// from reporting duplicated tags, keeping only the tags of the
	// collector with the higher priority, at whichever cardinality it
	// reports. we don't want two collectors running with the same priority
	// in the first place, so this code does not check for duplicates in
	// that case to keep code simpler.
	insertWithPriority := func(source string, tags []string, cardinality collectors.TagCardinality) {
		prio := collectors.CollectorPriorities[source]
		for _, t := range tags {
			tagName := strings.SplitN(t, ":", 2)[0]
			existingPrio, exists := tagMap[tagName]
			if exists && prio < existingPrio {
				continue
			}

			tagMap[tagName] = prio
			tagList[cardinality] = append(tagList[cardinality], t)
		}
	}

	for _, source := range sources {
		tags := e.sourceTags[source]
		insertWithPriority(source, tags.lowCardTags, collectors.LowCardinality)
		insertWithPriority(source, tags.orchestratorCardTags, collectors.OrchestratorCardinality)
		insertWithPriority(source, tags.highCardTags, collectors.HighCardinality)
	}

	numTags := len(tagList[collectors.LowCardinality])
	numTags += len(tagList[collectors.OrchestratorCardinality])
	numTags += len(tagList[collectors.HighCardinality])
	bldr := tagset.NewSliceBuilder(int(collectors.NumCardinalities), numTags)
	for card, tags := range tagList {
		for _, tag := range tags {
			bldr.Add(int(card), tag)
		}
	}

	// Write cache, including the various slices we might need
	e.cacheValid = true
	e.cachedAll = bldr.FreezeSlice(0, int(collectors.NumCardinalities))
	e.cachedHigh = bldr.FreezeSlice(int(collectors.HighCardinality), int(collectors.HighCardinality)+1)
	e.cachedOrchestrator = bldr.FreezeSlice(int(collectors.OrchestratorCardinality), int(collectors.OrchestratorCardinality)+1)
	e.cachedLow = bldr.FreezeSlice(int(collectors.LowCardinality), int(collectors.LowCardinality)+1)
	e.cachedLowOrchestrator = bldr.FreezeSlice(int(collectors.LowCardinality), int(collectors.OrchestratorCardinality)+1)
	bldr.Close()
}

func (e *EntityTags) shouldRemove() bool {
	for _, tags := range e.sourceTags {
		if !tags.expiryDate.IsZero() || !tags.isEmpty() {
			return false
		}
	}

	return true
}
