// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file defines the EntityTags interface which contains the tags for a
// tagger entity.
//
// There are two implementations of this interface:
// EntityTagsWithMultipleSources and EntityTagsWithSingleSource.
//
// EntityTagsWithMultipleSources is used when a tagger entity can be created
// from multiple sources. For example, when a container is reported by a node
// runtime like containerd and a node orchestrator like the kubelet. In this
// case, the implementation keeps the information from all sources, and it's able
// to merge them according to the priorities defined.
//
// EntityTagsWithSingleSource is used when a tagger entity is created from a
// single source. In this case, the implementation doesn't need to store the
// source information, reducing the memory footprint. EntityTagsWithSingleSource
// is only used in the Cluster Agent. The reason is that in the Cluster Agent
// the data can only come from static tags or a single workloadmeta collector
// (kubeapiserver), so an entity is never created from multiple sources.

// EntityTags holds the tag information for a given entity.
type EntityTags interface {
	toEntity() types.Entity
	getStandard() []string
	getHashedTags(cardinality types.TagCardinality) tagset.HashedTags
	tagsForSource(source string) *sourceTags
	tagsBySource() map[string][]string
	setTagsForSource(source string, tags sourceTags)
	sources() []string
	setSourceExpiration(source string, expiryDate time.Time)
	deleteExpired(time time.Time) bool
	shouldRemove() bool
}

// EntityTagsWithMultipleSources holds the tag information for a given entity
// that can be created from multiple sources. It is not thread-safe, so should
// not be shared outside of the store. Usage inside the store is safe since it
// relies on a global lock.
type EntityTagsWithMultipleSources struct {
	entityID           string
	sourceTags         map[string]sourceTags
	cacheValid         bool
	cachedAll          tagset.HashedTags // Low + orchestrator + high
	cachedOrchestrator tagset.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          tagset.HashedTags // Sub-slice of cachedAll
}

func newEntityTags(entityID string, source string) EntityTags {
	if flavor.GetFlavor() == flavor.ClusterAgent {
		return newEntityTagsWithSingleSource(entityID, source)
	}

	return &EntityTagsWithMultipleSources{
		entityID:   entityID,
		sourceTags: make(map[string]sourceTags),
		cacheValid: true,
	}
}

func (e *EntityTagsWithMultipleSources) toEntity() types.Entity {
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

func (e *EntityTagsWithMultipleSources) getStandard() []string {
	tags := []string{}
	for _, t := range e.sourceTags {
		tags = append(tags, t.standardTags...)
	}
	return tags
}

func (e *EntityTagsWithMultipleSources) getHashedTags(cardinality types.TagCardinality) tagset.HashedTags {
	e.computeCache()

	if cardinality == types.HighCardinality {
		return e.cachedAll
	} else if cardinality == types.OrchestratorCardinality {
		return e.cachedOrchestrator
	}
	return e.cachedLow
}

func (e *EntityTagsWithMultipleSources) computeCache() {
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

func (e *EntityTagsWithMultipleSources) deleteExpired(time time.Time) bool {
	initialNumSources := len(e.sourceTags)

	maps.DeleteFunc(e.sourceTags, func(_ string, tags sourceTags) bool {
		return tags.isExpired(time)
	})

	changed := len(e.sourceTags) != initialNumSources

	if changed {
		e.cacheValid = false
	}

	return changed
}

func (e *EntityTagsWithMultipleSources) shouldRemove() bool {
	for _, tags := range e.sourceTags {
		if !tags.expiryDate.IsZero() || !tags.isEmpty() {
			return false
		}
	}

	return true
}

func (e *EntityTagsWithMultipleSources) tagsForSource(source string) *sourceTags {
	tags, ok := e.sourceTags[source]
	if !ok {
		return nil
	}
	return &tags
}

func (e *EntityTagsWithMultipleSources) setTagsForSource(source string, tags sourceTags) {
	e.sourceTags[source] = tags
	e.cacheValid = false
}

func (e *EntityTagsWithMultipleSources) tagsBySource() map[string][]string {
	tagsBySource := make(map[string][]string)

	for source, tags := range e.sourceTags {
		allTags := append([]string{}, tags.lowCardTags...)
		allTags = append(allTags, tags.orchestratorCardTags...)
		allTags = append(allTags, tags.highCardTags...)
		tagsBySource[source] = allTags
	}

	return tagsBySource
}

func (e *EntityTagsWithMultipleSources) sources() []string {
	sources := make([]string, 0, len(e.sourceTags))
	for source := range e.sourceTags {
		sources = append(sources, source)
	}
	return sources
}

func (e *EntityTagsWithMultipleSources) setSourceExpiration(source string, expiryDate time.Time) {
	tags, ok := e.sourceTags[source]
	if !ok {
		return
	}

	tags.expiryDate = expiryDate
	e.sourceTags[source] = tags
}

// EntityTagsWithSingleSource holds the tag information for a given entity that
// can only be created from a single source. It is not thread-safe, so should
// not be shared outside of the store. Usage inside the store is safe since it
// relies on a global lock.
type EntityTagsWithSingleSource struct {
	entityID           string
	source             string
	expiryDate         time.Time
	standardTags       []string
	cachedAll          tagset.HashedTags // Low + orchestrator + high
	cachedOrchestrator tagset.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          tagset.HashedTags // Sub-slice of cachedAll
	isExpired          bool
}

func newEntityTagsWithSingleSource(entityID string, source string) *EntityTagsWithSingleSource {
	return &EntityTagsWithSingleSource{
		entityID: entityID,
		source:   source,
	}
}

func (e *EntityTagsWithSingleSource) toEntity() types.Entity {
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

func (e *EntityTagsWithSingleSource) getStandard() []string {
	return e.standardTags
}

func (e *EntityTagsWithSingleSource) getHashedTags(cardinality types.TagCardinality) tagset.HashedTags {
	switch cardinality {
	case types.HighCardinality:
		return e.cachedAll
	case types.OrchestratorCardinality:
		return e.cachedOrchestrator
	default:
		return e.cachedLow
	}
}

func (e *EntityTagsWithSingleSource) tagsForSource(source string) *sourceTags {
	if source != e.source {
		log.Errorf("Trying to get tags from source %s on entity with source %s", source, e.source)
		return nil
	}

	return &sourceTags{
		lowCardTags:          e.cachedLow.Get(),
		orchestratorCardTags: e.cachedAll.Slice(e.cachedLow.Len(), e.cachedOrchestrator.Len()).Get(),
		highCardTags:         e.cachedAll.Slice(e.cachedOrchestrator.Len(), e.cachedAll.Len()).Get(),
		standardTags:         e.standardTags,
		expiryDate:           e.expiryDate,
	}
}

func (e *EntityTagsWithSingleSource) tagsBySource() map[string][]string {
	return map[string][]string{e.source: e.cachedAll.Get()}
}

func (e *EntityTagsWithSingleSource) setTagsForSource(source string, tags sourceTags) {
	if source != e.source {
		log.Errorf("Trying to set tags for source %s on entity with source %s", source, e.source)
		return
	}

	e.standardTags = tags.standardTags

	all := make([]string, 0, len(tags.lowCardTags)+len(tags.orchestratorCardTags)+len(tags.highCardTags))
	all = append(all, tags.lowCardTags...)
	all = append(all, tags.orchestratorCardTags...)
	all = append(all, tags.highCardTags...)

	cached := tagset.NewHashedTagsFromSlice(all)

	e.cachedAll = cached
	e.cachedLow = cached.Slice(0, len(tags.lowCardTags))
	e.cachedOrchestrator = cached.Slice(0, len(tags.lowCardTags)+len(tags.orchestratorCardTags))
}

func (e *EntityTagsWithSingleSource) sources() []string {
	return []string{e.source}
}

func (e *EntityTagsWithSingleSource) setSourceExpiration(source string, expiryDate time.Time) {
	if source != e.source {
		log.Errorf("Trying to set expiration for source %s on entity with source %s", source, e.source)
		return
	}

	e.expiryDate = expiryDate
}

func (e *EntityTagsWithSingleSource) deleteExpired(time time.Time) bool {
	if !e.expiryDate.IsZero() && e.expiryDate.Before(time) {
		e.isExpired = true
		return true
	}

	return false
}

func (e *EntityTagsWithSingleSource) shouldRemove() bool {
	return e.isExpired || e.cachedAll.Len() == 0
}
