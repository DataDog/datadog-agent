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

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
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
	s.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:               "test",
			LowCardTags:          []string{"tag"},
			OrchestratorCardTags: []string{"tag"},
			HighCardTags:         []string{"tag"},
		},
		{
			Entity:      "test",
			LowCardTags: []string{"tag"},
		},
	})

	assert.Len(s.T(), s.store.store, 1)
}

func (s *StoreTestSuite) TestLookup() {
	s.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:               "test",
			LowCardTags:          []string{"tag"},
			HighCardTags:         []string{"tag"},
			OrchestratorCardTags: []string{"tag"},
		},
	})

	tagsHigh := s.store.Lookup("test", collectors.HighCardinality)
	tagsOrch := s.store.Lookup("test", collectors.OrchestratorCardinality)
	tagsLow := s.store.Lookup("test", collectors.LowCardinality)

	assert.Len(s.T(), tagsHigh, 3)
	assert.Len(s.T(), tagsLow, 1)
	assert.Len(s.T(), tagsOrch, 2)
}

func (s *StoreTestSuite) TestLookupStandard() {
	s.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "test",
			LowCardTags:  []string{"tag", "service:foo", "env:dev"},
			StandardTags: []string{"service:foo", "env:dev"},
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
	tags := s.store.Lookup("test", collectors.LowCardinality)
	assert.Nil(s.T(), tags)
}

func (s *StoreTestSuite) TestPrune__deletedEntities() {
	s.store.ProcessTagInfo([]*collectors.TagInfo{
		// Adds
		{
			Entity:               "test1",
			LowCardTags:          []string{"s1tag"},
			OrchestratorCardTags: []string{"s1tag"},
			HighCardTags:         []string{"s1tag"},
		},
		{
			Entity:       "test2",
			LowCardTags:  []string{"tag"},
			HighCardTags: []string{"tag"},
		},

		// Deletion, to be batched
		{
			Entity:       "test1",
			DeleteEntity: true,
		},
	})

	// Data should still be in the store
	tagsHigh := s.store.Lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 3)
	tagsOrch := s.store.Lookup("test1", collectors.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	tagsHigh = s.store.Lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)

	s.clock.Add(10 * time.Minute)
	s.store.Prune()

	// test1 should have been removed
	tagsHigh = s.store.Lookup("test1", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 0)

	// test2 should still be present
	tagsHigh = s.store.Lookup("test2", collectors.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}

func (s *StoreTestSuite) TestGetExpiredTags() {
	s.store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "entityA",
			HighCardTags: []string{"expired"},
			ExpiryDate:   s.clock.Now().Add(-10 * time.Second),
		},
		{
			Entity:       "entityB",
			HighCardTags: []string{"expiresSoon"},
			ExpiryDate:   s.clock.Now().Add(10 * time.Second),
		},
	})

	s.store.Prune()

	tagsHigh := s.store.Lookup("entityB", collectors.HighCardinality)
	assert.Contains(s.T(), tagsHigh, "expiresSoon")

	tagsHigh = s.store.Lookup("entityA", collectors.HighCardinality)
	assert.NotContains(s.T(), tagsHigh, "expired")
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

	var expectedEvents = []entityEventExpectation{
		{types.EventTypeAdded, "test1", []string{"low"}, []string{}, []string{"high:1"}},
		{types.EventTypeModified, "test1", []string{"low"}, []string{"orch"}, []string{"high:1", "high:2"}},
		{types.EventTypeAdded, "test2", []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeDeleted, "test1", nil, nil, nil},
	}

	store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:       "test1",
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high:1"},
		},
	})

	highCardEvents := []types.EntityEvent{}
	lowCardEvents := []types.EntityEvent{}

	highCardCh := store.Subscribe(collectors.HighCardinality)
	lowCardCh := store.Subscribe(collectors.LowCardinality)

	store.ProcessTagInfo([]*collectors.TagInfo{
		{
			Entity:               "test1",
			LowCardTags:          []string{"low"},
			OrchestratorCardTags: []string{"orch"},
			HighCardTags:         []string{"high:1", "high:2"},
		},
		{
			Entity:       "test2",
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	store.ProcessTagInfo([]*collectors.TagInfo{
		{
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

	checkEvents(t, expectedEvents, lowCardEvents, collectors.LowCardinality)
	checkEvents(t, expectedEvents, highCardEvents, collectors.HighCardinality)
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

		assert.Equal(t, expectation.lowCardTags, event.Entity.LowCardinalityTags, "event %d, low cardinality", i)
		if cardinality == collectors.OrchestratorCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags, "event %d, orch cardinality", i)
			assert.Empty(t, event.Entity.HighCardinalityTags, "event %d, high cardinality", i)
		} else if cardinality == collectors.HighCardinality {
			assert.Equal(t, expectation.orchCardTags, event.Entity.OrchestratorCardinalityTags, "event %d, orch cardinality", i)
			assert.Equal(t, expectation.highCardTags, event.Entity.HighCardinalityTags, "event %d, high cardinality", i)
		} else {
			assert.Empty(t, event.Entity.OrchestratorCardinalityTags, "event %d, orch cardinality", i)
			assert.Empty(t, event.Entity.HighCardinalityTags, "event %d, high cardinality", i)
		}
	}
}
