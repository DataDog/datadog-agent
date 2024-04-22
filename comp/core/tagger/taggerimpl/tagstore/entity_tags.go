// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// EntityTags holds the tag information for a given entity. It is not
// thread-safe, so should not be shared outside of the store. Usage inside the
// store is safe since it relies on a global lock.
type EntityTags struct {
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedAll          tagset.HashedTags // Low + orchestrator + high
	cachedOrchestrator tagset.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          tagset.HashedTags // Sub-slice of cachedAll
}

func newEntityTags(entityID string) *EntityTags {
	return &EntityTags{
		entityID:   entityID,
		sourceTags: make(map[string]sourceTags),
		cacheValid: true,
	}
}

func (e *EntityTags) getStandard() []string {
	tags := []string{}
	for _, t := range e.sourceTags {
		tags = append(tags, t.standardTags...)
	}
	return tags
}

func (e *EntityTags) get(cardinality types.TagCardinality) []string {
	return e.getHashedTags(cardinality).Get()
}

func (e *EntityTags) getHashedTags(cardinality types.TagCardinality) tagset.HashedTags {
	e.computeCache()

	if cardinality == types.HighCardinality {
		return e.cachedAll
	} else if cardinality == types.OrchestratorCardinality {
		return e.cachedOrchestrator
	}
	return e.cachedLow
}

func (e *EntityTags) toEntity() types.Entity {
	e.computeCache()

	cachedAll := e.cachedAll.Get()
	cachedOrchestrator := e.cachedOrchestrator.Get()
	cachedLow := e.cachedLow.Get()

	return types.Entity{
		ID:           e.entityID,
		StandardTags: e.getStandard(),
		// cachedAll contains low, orchestrator and high cardinality tags, in this order.
		// cachedOrchestrator and cachedLow are subslices of cachedAll, starting at index 0.
		HighCardinalityTags:         cachedAll[len(cachedOrchestrator):],
		OrchestratorCardinalityTags: cachedOrchestrator[len(cachedLow):],
		LowCardinalityTags:          cachedLow,
	}
}

func (e *EntityTags) computeCache() {
	if e.cacheValid {
		return
	}

	tagList := make(map[types.TagCardinality][]string)
	tagMap := make(map[string]types.CollectorPriority)

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
	insertWithPriority := func(source string, tags []string, cardinality types.TagCardinality) {
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
		insertWithPriority(source, tags.lowCardTags, types.LowCardinality)
		insertWithPriority(source, tags.orchestratorCardTags, types.OrchestratorCardinality)
		insertWithPriority(source, tags.highCardTags, types.HighCardinality)
	}

	tags := append(tagList[types.LowCardinality], tagList[types.OrchestratorCardinality]...)
	tags = append(tags, tagList[types.HighCardinality]...)

	cached := tagset.NewHashedTagsFromSlice(tags)

	lowCardTags := len(tagList[types.LowCardinality])
	orchCardTags := len(tagList[types.OrchestratorCardinality])

	// Write cache
	e.cacheValid = true
	e.cachedAll = cached
	e.cachedLow = cached.Slice(0, lowCardTags)
	e.cachedOrchestrator = cached.Slice(0, lowCardTags+orchCardTags)
}

func (e *EntityTags) shouldRemove() bool {
	for _, tags := range e.sourceTags {
		if !tags.expiryDate.IsZero() || !tags.isEmpty() {
			return false
		}
	}

	return true
}
