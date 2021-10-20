package tagstore

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

func (e *EntityTags) get(cardinality collectors.TagCardinality) []string {
	return e.getHashedTags(cardinality).Get()
}

func (e *EntityTags) getHashedTags(cardinality collectors.TagCardinality) tagset.HashedTags {
	e.computeCache()

	if cardinality == collectors.HighCardinality {
		return e.cachedAll
	} else if cardinality == collectors.OrchestratorCardinality {
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

type tagPriority struct {
	tag         string                       // full tag
	priority    collectors.CollectorPriority // collector priority
	cardinality collectors.TagCardinality    // cardinality level of the tag (low, orchestrator, high)
}

func (e *EntityTags) computeCache() {
	if e.cacheValid {
		return
	}

	var sources []string
	tagPrioMapper := make(map[string][]tagPriority)

	for source, tags := range e.sourceTags {
		sources = append(sources, source)
		insertWithPriority(tagPrioMapper, tags.lowCardTags, source, collectors.LowCardinality)
		insertWithPriority(tagPrioMapper, tags.orchestratorCardTags, source, collectors.OrchestratorCardinality)
		insertWithPriority(tagPrioMapper, tags.highCardTags, source, collectors.HighCardinality)
	}

	var lowCardTags []string
	var orchestratorCardTags []string
	var highCardTags []string
	for _, tags := range tagPrioMapper {
		for i := 0; i < len(tags); i++ {
			insert := true
			for j := 0; j < len(tags); j++ {
				// if we find a duplicate tag with higher priority we do not insert the tag
				if i != j && tags[i].priority < tags[j].priority {
					insert = false
					break
				}
			}
			if !insert {
				continue
			}
			if tags[i].cardinality == collectors.HighCardinality {
				highCardTags = append(highCardTags, tags[i].tag)
				continue
			} else if tags[i].cardinality == collectors.OrchestratorCardinality {
				orchestratorCardTags = append(orchestratorCardTags, tags[i].tag)
				continue
			}
			lowCardTags = append(lowCardTags, tags[i].tag)
		}
	}

	tags := append(lowCardTags, orchestratorCardTags...)
	tags = append(tags, highCardTags...)

	cached := tagset.NewHashedTagsFromSlice(tags)

	// Write cache
	e.cacheValid = true
	e.cachedAll = cached
	e.cachedLow = cached.Slice(0, len(lowCardTags))
	e.cachedOrchestrator = cached.Slice(0, len(lowCardTags)+len(orchestratorCardTags))
}

func (e *EntityTags) shouldRemove() bool {
	for _, tags := range e.sourceTags {
		if !tags.expiryDate.IsZero() || !tags.isEmpty() {
			return false
		}
	}

	return true
}

func insertWithPriority(tagPrioMapper map[string][]tagPriority, tags []string, source string, cardinality collectors.TagCardinality) {
	priority, found := collectors.CollectorPriorities[source]
	if !found {
		log.Warnf("Tagger: %s collector has no defined priority, assuming low", source)
		priority = collectors.NodeRuntime
	}

	for _, t := range tags {
		tagName := strings.Split(t, ":")[0]
		tagPrioMapper[tagName] = append(tagPrioMapper[tagName], tagPriority{
			tag:         t,
			priority:    priority,
			cardinality: cardinality,
		})
	}
}
