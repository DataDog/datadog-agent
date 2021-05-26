// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package local

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
)

type StoreTestSuite struct {
	suite.Suite
	store *tagStore
}

func (s *StoreTestSuite) SetupTest() {
	s.store = newTagStore()
}

func (s *StoreTestSuite) TestIngest() {
	s.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:               "source1",
			Entity:               "test",
			LowCardTags:          []string{"tag"},
			OrchestratorCardTags: []string{"tag"},
			HighCardTags:         []string{"tag"},
		},
		{
			Source:      "source2",
			Entity:      "test",
			LowCardTags: []string{"tag"},
		},
	})

	assert.Len(s.T(), s.store.store, 1)
	assert.Len(s.T(), s.store.store["test"].sourceTags, 2)
}

func (s *StoreTestSuite) TestLookup() {
	s.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:       "source1",
			Entity:       "test",
			LowCardTags:  []string{"tag"},
			HighCardTags: []string{"tag"},
		},
		{
			Source:      "source2",
			Entity:      "test",
			LowCardTags: []string{"tag"},
		},
		{
			Source:               "source3",
			Entity:               "test",
			OrchestratorCardTags: []string{"tag"},
		},
	})

	tagsHigh, sourcesHigh := s.store.lookup("test", collectors.HighCardinality)
	tagsOrch, sourcesOrch := s.store.lookup("test", collectors.OrchestratorCardinality)
	tagsLow, sourcesLow := s.store.lookup("test", collectors.LowCardinality)

	assert.Len(s.T(), tagsHigh, 4)
	assert.Len(s.T(), tagsLow, 2)
	assert.Len(s.T(), tagsOrch, 3)

	assert.Len(s.T(), sourcesHigh, 3)
	assert.Contains(s.T(), sourcesHigh, "source1")
	assert.Contains(s.T(), sourcesHigh, "source2")
	assert.Contains(s.T(), sourcesHigh, "source3")

	assert.Len(s.T(), sourcesOrch, 3)
	assert.Contains(s.T(), sourcesOrch, "source1")
	assert.Contains(s.T(), sourcesOrch, "source2")
	assert.Contains(s.T(), sourcesOrch, "source3")

	assert.Len(s.T(), sourcesLow, 3)
	assert.Contains(s.T(), sourcesLow, "source1")
	assert.Contains(s.T(), sourcesLow, "source2")
	assert.Contains(s.T(), sourcesLow, "source3")
}

func (s *StoreTestSuite) TestLookupStandard() {
	s.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:       "source1",
			Entity:       "test",
			LowCardTags:  []string{"tag", "env:dev"},
			StandardTags: []string{"env:dev"},
		},
		{
			Source:       "source2",
			Entity:       "test",
			LowCardTags:  []string{"tag", "service:foo"},
			StandardTags: []string{"service:foo"},
		},
	})

	standard, err := s.store.lookupStandard("test")
	assert.Nil(s.T(), err)
	assert.Len(s.T(), standard, 2)
	assert.Contains(s.T(), standard, "env:dev")
	assert.Contains(s.T(), standard, "service:foo")

	_, err = s.store.lookupStandard("not found")
	assert.NotNil(s.T(), err)
}

func (s *StoreTestSuite) TestLookupNotPresent() {
	tags, sources := s.store.lookup("test", collectors.LowCardinality)
	assert.Nil(s.T(), tags)
	assert.Nil(s.T(), sources)
}

func (s *StoreTestSuite) TestPruneDeletedEntities() {
	s.store.processTagInfo([]*collectors.TagInfo{
		// Adds
		{
			Source:               "source1",
			Entity:               "test1",
			LowCardTags:          []string{"s1tag"},
			OrchestratorCardTags: []string{"s1tag"},
			HighCardTags:         []string{"s1tag"},
		},
		{
			Source:       "source2",
			Entity:       "test1",
			HighCardTags: []string{"s2tag"},
		},
		{
			Source:       "source1",
			Entity:       "test2",
			LowCardTags:  []string{"tag"},
			HighCardTags: []string{"tag"},
		},

		// Deletion, to be batched
		{
			Source:       "source1",
			Entity:       "test1",
			DeleteEntity: true,
		},
	})

	assert.Len(s.T(), s.store.toDelete, 1)

	// Data should still be in the store
	tagsHigh, sourcesHigh := s.store.lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 4)
	assert.Len(s.T(), sourcesHigh, 2)
	tagsOrch, sourcesOrch := s.store.lookup("test1", collectors.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	assert.Len(s.T(), sourcesOrch, 2)
	tagsHigh, sourcesHigh = s.store.lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	s.store.pruneDeletedEntities()

	// deletion map should be empty now
	assert.Len(s.T(), s.store.toDelete, 0)

	// test1 should only have tags from source2, source1 should be removed
	tagsHigh, sourcesHigh = s.store.lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	assert.Len(s.T(), sourcesHigh, 1)
	tagsOrch, sourcesOrch = s.store.lookup("test1", collectors.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 0)
	assert.Len(s.T(), sourcesOrch, 1)

	// test2 should still be present
	tagsHigh, sourcesHigh = s.store.lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	s.store.processTagInfo([]*collectors.TagInfo{
		// re-add tags from removed source, then remove another one
		{
			Source:      "source1",
			Entity:      "test1",
			LowCardTags: []string{"s1tag"},
		},
		// Deletion, to be batched
		{
			Source:       "source2",
			Entity:       "test1",
			DeleteEntity: true,
		},
	})

	s.store.pruneDeletedEntities()

	tagsHigh, sourcesHigh = s.store.lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	assert.Len(s.T(), sourcesHigh, 1)
	tagsHigh, sourcesHigh = s.store.lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)
}

func (s *StoreTestSuite) TestPruneEmptyEntries() {
	s.store.processTagInfo([]*collectors.TagInfo{
		{
			Source:               "source1",
			Entity:               "test1",
			LowCardTags:          []string{"s1tag"},
			OrchestratorCardTags: []string{"s1tag"},
			HighCardTags:         []string{"s1tag"},
		},
		{
			Source:       "source2",
			Entity:       "test2",
			HighCardTags: []string{"s2tag"},
		},
		{
			Source:      "emptySource1",
			Entity:      "emptyEntity1",
			LowCardTags: []string{},
		},
		{
			Source:       "emptySource2",
			Entity:       "emptyEntity2",
			StandardTags: []string{},
		},
		{
			Source:      "emptySource3",
			Entity:      "test3",
			LowCardTags: []string{},
		},
		{
			Source:      "source3",
			Entity:      "test3",
			LowCardTags: []string{"s3tag"},
		},
	})

	assert.Len(s.T(), s.store.store, 5)
	s.store.pruneEmptyEntries()
	assert.Len(s.T(), s.store.store, 3)

	// Assert non-empty tags aren't deleted
	tagsHigh, sourcesHigh := s.store.lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 3)
	assert.Len(s.T(), sourcesHigh, 1)
	tagsOrch, sourcesOrch := s.store.lookup("test1", collectors.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	assert.Len(s.T(), sourcesOrch, 1)
	tagsHigh, sourcesHigh = s.store.lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	assert.Len(s.T(), sourcesHigh, 1)
	tagsLow, sourcesLow := s.store.lookup("test3", collectors.LowCardinality)
	assert.Len(s.T(), tagsLow, 1)
	assert.Len(s.T(), sourcesLow, 2)

	// Assert empty entities are deleted
	emptyTags1, emptySource1 := s.store.lookup("emptyEntity1", collectors.HighCardinality)
	assert.Len(s.T(), emptyTags1, 0)
	assert.Len(s.T(), emptySource1, 0)
	emptyTags2, emptySource2 := s.store.lookup("emptyEntity2", collectors.HighCardinality)
	assert.Len(s.T(), emptyTags2, 0)
	assert.Len(s.T(), emptySource2, 0)
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}

func TestGetEntityTags(t *testing.T) {
	etags := newEntityTags("deadbeef")

	// Get empty tags and make sure cache is now set to valid
	tags, sources := etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Add tags but don't invalidate the cache, we should return empty arrays
	etags.sourceTags["source"] = sourceTags{
		lowCardTags:  []string{"low1", "low2"},
		highCardTags: []string{"high1", "high2"},
	}
	tags, sources = etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags, sources = etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 4)
	assert.ElementsMatch(t, tags, []string{"low1", "low2", "high1", "high2"})
	assert.Len(t, sources, 1)
	assert.True(t, etags.cacheValid)
	tags, sources = etags.get(collectors.LowCardinality)
	assert.Len(t, tags, 2)
	assert.ElementsMatch(t, tags, []string{"low1", "low2"})
	assert.Len(t, sources, 1)
}

func TestDuplicateSourceTags(t *testing.T) {
	etags := newEntityTags("deadbeef")

	// Get empty tags and make sure cache is now set to valid
	tags, sources := etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Mock collector priorities
	collectors.CollectorPriorities = map[string]collectors.CollectorPriority{
		"sourceNodeOrchestrator":    collectors.NodeOrchestrator,
		"sourceNodeRuntime":         collectors.NodeRuntime,
		"sourceClusterOrchestrator": collectors.ClusterOrchestrator,
	}

	// Add tags but don't invalidate the cache, we should return empty arrays
	etags.sourceTags["sourceNodeOrchestrator"] = sourceTags{
		lowCardTags:  []string{"bar", "tag1:sourceHigh", "tag2:sourceHigh"},
		highCardTags: []string{"tag3:sourceHigh", "tag4:sourceHigh"},
	}

	etags.sourceTags["sourceNodeRuntime"] = sourceTags{
		lowCardTags:  []string{"foo", "tag1:sourceLow", "tag2:sourceLow"},
		highCardTags: []string{"tag3:sourceLow", "tag5:sourceLow"},
	}

	etags.sourceTags["sourceClusterOrchestrator"] = sourceTags{
		lowCardTags:  []string{"tag3:sourceClusterHigh", "tag1:sourceClusterLow"},
		highCardTags: []string{"tag4:sourceClusterLow"},
	}

	tags, sources = etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags, sources = etags.get(collectors.HighCardinality)
	assert.Len(t, tags, 7)
	assert.ElementsMatch(t, tags, []string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh", "tag4:sourceClusterLow", "tag5:sourceLow"})
	assert.Len(t, sources, 3)
	assert.True(t, etags.cacheValid)
	tags, sources = etags.get(collectors.LowCardinality)
	assert.Len(t, sources, 3)
	assert.Len(t, tags, 5)
	assert.ElementsMatch(t, tags, []string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh"})
}

type entityEventExpectation struct {
	eventType    types.EventType
	id           string
	lowCardTags  []string
	orchCardTags []string
	highCardTags []string
}

func TestSubscribe(t *testing.T) {
	store := newTagStore()

	collectors.CollectorPriorities["source2"] = collectors.ClusterOrchestrator

	var expectedEvents = []entityEventExpectation{
		{types.EventTypeAdded, "test1", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, "test1", []string{"low"}, []string{"orch"}, []string{"high:1", "high:2"}},
		{types.EventTypeAdded, "test2", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, "test1", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeDeleted, "test1", nil, nil, nil},
	}

	store.processTagInfo([]*collectors.TagInfo{
		{
			Source:       "source",
			Entity:       "test1",
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	highCardEvents := []types.EntityEvent{}
	lowCardEvents := []types.EntityEvent{}

	highCardCh := store.subscribe(collectors.HighCardinality)
	lowCardCh := store.subscribe(collectors.LowCardinality)

	store.processTagInfo([]*collectors.TagInfo{
		{
			Source:               "source2",
			Entity:               "test1",
			LowCardTags:          []string{"low"},
			OrchestratorCardTags: []string{"orch"},
			HighCardTags:         []string{"high:1", "high:2"},
		},
		{
			Source:       "source2",
			Entity:       "test1",
			DeleteEntity: true,
		},
		{
			Source:       "source",
			Entity:       "test2",
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	store.pruneDeletedEntities()

	store.processTagInfo([]*collectors.TagInfo{
		{
			Source:       "source",
			Entity:       "test1",
			DeleteEntity: true,
		},
	})

	store.pruneDeletedEntities()

	var wg sync.WaitGroup
	wg.Add(2)

	go collectEvents(&wg, &highCardEvents, highCardCh)
	go collectEvents(&wg, &lowCardEvents, lowCardCh)

	store.unsubscribe(highCardCh)
	store.unsubscribe(lowCardCh)

	wg.Wait()

	checkEvents(t, expectedEvents, highCardEvents, collectors.HighCardinality)
	checkEvents(t, expectedEvents, lowCardEvents, collectors.LowCardinality)
}

func collectEvents(wg *sync.WaitGroup, events *[]types.EntityEvent, ch chan []types.EntityEvent) {
	for chEvents := range ch {
		for _, event := range chEvents {
			*events = append(*events, event)
		}
	}

	wg.Done()
}

func checkEvents(t *testing.T, expectations []entityEventExpectation, events []types.EntityEvent, cardinality collectors.TagCardinality) {
	passed := assert.Len(t, events, len(expectations))
	if !passed {
		return
	}

	for i, expectation := range expectations {
		event := events[i]

		passed = assert.Equal(t, expectation.eventType, event.EventType)
		passed = passed && assert.Equal(t, expectation.id, event.Entity.ID)
		if !passed {
			return
		}

		assert.Equal(t, expectation.lowCardTags, event.Entity.LowCardinalityTags)
		if cardinality == collectors.OrchestratorCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags)
			assert.Empty(t, event.Entity.HighCardinalityTags)
		} else if cardinality == collectors.HighCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags)
			assert.Equal(t, expectation.highCardTags, event.Entity.HighCardinalityTags)
		} else {
			assert.Empty(t, event.Entity.OrchestratorCardinalityTags)
			assert.Empty(t, event.Entity.HighCardinalityTags)
		}
	}
}
