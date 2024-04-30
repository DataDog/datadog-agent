// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

type StoreTestSuite struct {
	suite.Suite
	clock *clock.Mock
	store *TagStore
}

func (s *StoreTestSuite) SetupTest() {
	s.clock = clock.NewMock()
	// set the mock clock to the current time
	s.clock.Add(time.Since(time.Unix(0, 0)))
	s.store = newTagStoreWithClock(s.clock)
}

func (s *StoreTestSuite) TestIngest() {
	s.store.ProcessTagInfo([]*types.TagInfo{
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
	s.store.ProcessTagInfo([]*types.TagInfo{
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

	tagsHigh := s.store.Lookup("test", types.HighCardinality)
	tagsOrch := s.store.Lookup("test", types.OrchestratorCardinality)
	tagsLow := s.store.Lookup("test", types.LowCardinality)

	assert.Len(s.T(), tagsHigh, 4)
	assert.Len(s.T(), tagsLow, 2)
	assert.Len(s.T(), tagsOrch, 3)
}

func (s *StoreTestSuite) TestLookupStandard() {
	s.store.ProcessTagInfo([]*types.TagInfo{
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

	standard, err := s.store.LookupStandard("test")
	assert.Nil(s.T(), err)
	assert.Len(s.T(), standard, 2)
	assert.Contains(s.T(), standard, "env:dev")
	assert.Contains(s.T(), standard, "service:foo")

	_, err = s.store.LookupStandard("not found")
	assert.NotNil(s.T(), err)
}

func (s *StoreTestSuite) TestLookupNotPresent() {
	tags := s.store.Lookup("test", types.LowCardinality)
	assert.Nil(s.T(), tags)
}

func (s *StoreTestSuite) TestPrune__deletedEntities() {
	s.store.ProcessTagInfo([]*types.TagInfo{
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

	// Data should still be in the store
	tagsHigh := s.store.Lookup("test1", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 4)
	tagsOrch := s.store.Lookup("test1", types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	tagsHigh = s.store.Lookup("test2", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)

	s.clock.Add(10 * time.Minute)
	s.store.Prune()

	// test1 should only have tags from source2, source1 should be removed
	tagsHigh = s.store.Lookup("test1", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsOrch = s.store.Lookup("test1", types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 0)

	// test2 should still be present
	tagsHigh = s.store.Lookup("test2", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)

	s.store.ProcessTagInfo([]*types.TagInfo{
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

	s.clock.Add(10 * time.Minute)
	s.store.Prune()

	tagsHigh = s.store.Lookup("test1", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsHigh = s.store.Lookup("test2", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
}

func (s *StoreTestSuite) TestPrune__emptyEntries() {
	s.store.ProcessTagInfo([]*types.TagInfo{
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
	s.store.Prune()
	assert.Len(s.T(), s.store.store, 3)

	// Assert non-empty tags aren't deleted
	tagsHigh := s.store.Lookup("test1", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 3)
	tagsOrch := s.store.Lookup("test1", types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	tagsHigh = s.store.Lookup("test2", types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsLow := s.store.Lookup("test3", types.LowCardinality)
	assert.Len(s.T(), tagsLow, 1)

	// Assert empty entities are deleted
	emptyTags1 := s.store.Lookup("emptyEntity1", types.HighCardinality)
	assert.Len(s.T(), emptyTags1, 0)
	emptyTags2 := s.store.Lookup("emptyEntity2", types.HighCardinality)
	assert.Len(s.T(), emptyTags2, 0)
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}

func TestGetEntityTags(t *testing.T) {
	etags := newEntityTags("deadbeef")

	// Get empty tags and make sure cache is now set to valid
	tags := etags.get(types.HighCardinality)
	assert.Len(t, tags, 0)
	assert.True(t, etags.cacheValid)

	// Add tags but don't invalidate the cache, we should return empty arrays
	etags.sourceTags["source"] = sourceTags{
		lowCardTags:  []string{"low1", "low2"},
		highCardTags: []string{"high1", "high2"},
	}
	tags = etags.get(types.HighCardinality)
	assert.Len(t, tags, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags = etags.get(types.HighCardinality)
	assert.Len(t, tags, 4)
	assert.ElementsMatch(t, tags, []string{"low1", "low2", "high1", "high2"})
	assert.True(t, etags.cacheValid)
	tags = etags.get(types.LowCardinality)
	assert.Len(t, tags, 2)
	assert.ElementsMatch(t, tags, []string{"low1", "low2"})
}

func (s *StoreTestSuite) TestGetExpiredTags() {
	s.store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			Entity:       "entityA",
			HighCardTags: []string{"expired"},
			ExpiryDate:   s.clock.Now().Add(-10 * time.Second),
		},
		{
			Source:       "source",
			Entity:       "entityB",
			HighCardTags: []string{"expiresSoon"},
			ExpiryDate:   s.clock.Now().Add(10 * time.Second),
		},
	})

	s.store.Prune()

	tagsHigh := s.store.Lookup("entityB", types.HighCardinality)
	assert.Contains(s.T(), tagsHigh, "expiresSoon")

	tagsHigh = s.store.Lookup("entityA", types.HighCardinality)
	assert.NotContains(s.T(), tagsHigh, "expired")
}

func TestDuplicateSourceTags(t *testing.T) {
	etags := newEntityTags("deadbeef")

	// Get empty tags and make sure cache is now set to valid
	tags := etags.get(types.HighCardinality)
	assert.Len(t, tags, 0)
	assert.True(t, etags.cacheValid)

	// Mock collector priorities
	collectors.CollectorPriorities = map[string]types.CollectorPriority{
		"sourceNodeOrchestrator":    types.NodeOrchestrator,
		"sourceNodeRuntime":         types.NodeRuntime,
		"sourceClusterOrchestrator": types.ClusterOrchestrator,
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

	tags = etags.get(types.HighCardinality)
	assert.Len(t, tags, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags = etags.get(types.HighCardinality)
	assert.Len(t, tags, 7)
	assert.ElementsMatch(t, tags, []string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh", "tag4:sourceClusterLow", "tag5:sourceLow"})
	assert.True(t, etags.cacheValid)
	tags = etags.get(types.LowCardinality)
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
	clock := clock.NewMock()
	store := newTagStoreWithClock(clock)

	collectors.CollectorPriorities["source2"] = types.ClusterOrchestrator
	collectors.CollectorPriorities["source"] = types.NodeRuntime

	var expectedEvents = []entityEventExpectation{
		{types.EventTypeAdded, "test1", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, "test1", []string{"low"}, []string{"orch"}, []string{"high:1", "high:2"}},
		{types.EventTypeAdded, "test2", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, "test1", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeDeleted, "test1", nil, nil, nil},
	}

	store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			Entity:       "test1",
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	highCardEvents := []types.EntityEvent{}
	lowCardEvents := []types.EntityEvent{}

	highCardCh := store.Subscribe(types.HighCardinality)
	lowCardCh := store.Subscribe(types.LowCardinality)

	store.ProcessTagInfo([]*types.TagInfo{
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

	clock.Add(10 * time.Minute)
	store.Prune()

	store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			Entity:       "test1",
			DeleteEntity: true,
		},
	})

	clock.Add(10 * time.Minute)
	store.Prune()

	var wg sync.WaitGroup
	wg.Add(2)

	go collectEvents(&wg, &highCardEvents, highCardCh)
	go collectEvents(&wg, &lowCardEvents, lowCardCh)

	store.Unsubscribe(highCardCh)
	store.Unsubscribe(lowCardCh)

	wg.Wait()

	checkEvents(t, expectedEvents, highCardEvents, types.HighCardinality)
	checkEvents(t, expectedEvents, lowCardEvents, types.LowCardinality)
}

func collectEvents(wg *sync.WaitGroup, events *[]types.EntityEvent, ch chan []types.EntityEvent) {
	for chEvents := range ch {
		*events = append(*events, chEvents...)
	}

	wg.Done()
}

func checkEvents(t *testing.T, expectations []entityEventExpectation, events []types.EntityEvent, cardinality types.TagCardinality) {
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
		if cardinality == types.OrchestratorCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags)
			assert.Empty(t, event.Entity.HighCardinalityTags)
		} else if cardinality == types.HighCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags)
			assert.Equal(t, expectation.highCardTags, event.Entity.HighCardinalityTags)
		} else {
			assert.Empty(t, event.Entity.OrchestratorCardinalityTags)
			assert.Empty(t, event.Entity.HighCardinalityTags)
		}
	}
}
