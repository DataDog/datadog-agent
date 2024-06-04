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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type StoreTestSuite struct {
	suite.Suite
	clock *clock.Mock
	store *TagStore
}

func (s *StoreTestSuite) SetupTest() {
	tel := fxutil.Test[telemetry.Component](s.T(), nooptelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	s.clock = clock.NewMock()
	// set the mock clock to the current time
	s.clock.Add(time.Since(time.Unix(0, 0)))
	s.store = newTagStoreWithClock(s.clock, telemetryStore)
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
	assert.Len(s.T(), s.store.store["test"].sources(), 2)
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

func (s *StoreTestSuite) TestList() {
	s.store.ProcessTagInfo(
		[]*types.TagInfo{
			{
				Source:               "source-1",
				Entity:               "entity-1",
				HighCardTags:         []string{"h1:v1", "h2:v2"},
				OrchestratorCardTags: []string{"o1:v1", "o2:v2"},
				LowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
			{
				Source:               "source-1",
				Entity:               "entity-2",
				HighCardTags:         []string{"h3:v3", "h4:v4"},
				OrchestratorCardTags: []string{"o3:v3", "o4:v4"},
				LowCardTags:          []string{"l3:v3", "l4:v4", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
		},
	)

	resultList := s.store.List()
	require.Equal(s.T(), 2, len(resultList.Entities))

	entity1, ok := resultList.Entities["entity-1"]
	require.True(s.T(), ok)
	require.Equal(s.T(), 1, len(entity1.Tags))
	require.ElementsMatch( // Tags order is not important
		s.T(),
		entity1.Tags["source-1"],
		[]string{"l1:v1", "l2:v2", "service:s1", "o1:v1", "o2:v2", "h1:v1", "h2:v2"},
	)

	entity2, ok := resultList.Entities["entity-2"]
	require.True(s.T(), ok)
	require.Equal(s.T(), 1, len(entity2.Tags))
	require.ElementsMatch( // Tags order is not important
		s.T(),
		entity2.Tags["source-1"],
		[]string{"l3:v3", "l4:v4", "service:s1", "o3:v3", "o4:v4", "h3:v3", "h4:v4"},
	)
}

func (s *StoreTestSuite) TestGetEntity() {
	_, err := s.store.GetEntity("entity-1")
	require.Error(s.T(), err)

	s.store.ProcessTagInfo(
		[]*types.TagInfo{
			{
				Source:               "source-1",
				Entity:               "entity-1",
				HighCardTags:         []string{"h1:v1", "h2:v2"},
				OrchestratorCardTags: []string{"o1:v1", "o2:v2"},
				LowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
		},
	)

	entity, err := s.store.GetEntity("entity-1")
	require.NoError(s.T(), err)
	assert.Equal(
		s.T(),
		&types.Entity{
			ID:                          "entity-1",
			HighCardinalityTags:         []string{"h1:v1", "h2:v2"},
			OrchestratorCardinalityTags: []string{"o1:v1", "o2:v2"},
			LowCardinalityTags:          []string{"l1:v1", "l2:v2", "service:s1"},
			StandardTags:                []string{"service:s1"},
		},
		entity,
	)
}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
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

func (s *StoreTestSuite) TestDuplicateSourceTags() {
	// Mock collector priorities
	originalCollectorPriorities := collectors.CollectorPriorities
	collectors.CollectorPriorities = map[string]types.CollectorPriority{
		"sourceNodeOrchestrator":    types.NodeOrchestrator,
		"sourceNodeRuntime":         types.NodeRuntime,
		"sourceClusterOrchestrator": types.ClusterOrchestrator,
	}
	defer func() {
		collectors.CollectorPriorities = originalCollectorPriorities
	}()

	testEntity := "testEntity"

	// Mock collector priorities
	collectors.CollectorPriorities = map[string]types.CollectorPriority{
		"sourceNodeRuntime":         types.NodeRuntime,
		"sourceNodeOrchestrator":    types.NodeOrchestrator,
		"sourceClusterOrchestrator": types.ClusterOrchestrator,
	}

	nodeRuntimeTags := types.TagInfo{
		Source:       "sourceNodeRuntime",
		Entity:       testEntity,
		LowCardTags:  []string{"foo", "tag1:sourceLow", "tag2:sourceLow"},
		HighCardTags: []string{"tag3:sourceLow", "tag5:sourceLow"},
	}

	nodeOrchestractorTags := types.TagInfo{
		Source:       "sourceNodeOrchestrator",
		Entity:       testEntity,
		LowCardTags:  []string{"bar", "tag1:sourceHigh", "tag2:sourceHigh"},
		HighCardTags: []string{"tag3:sourceHigh", "tag4:sourceHigh"},
	}

	clusterOrchestratorTags := types.TagInfo{
		Source:       "sourceClusterOrchestrator",
		Entity:       testEntity,
		LowCardTags:  []string{"tag1:sourceClusterLow", "tag3:sourceClusterHigh"},
		HighCardTags: []string{"tag4:sourceClusterLow"},
	}

	s.store.ProcessTagInfo([]*types.TagInfo{
		&nodeRuntimeTags,
		&nodeOrchestractorTags,
		&clusterOrchestratorTags,
	})

	lowCardTags := s.store.Lookup(testEntity, types.LowCardinality)
	assert.ElementsMatch(
		s.T(),
		lowCardTags,
		[]string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh"},
	)

	highCardTags := s.store.Lookup(testEntity, types.HighCardinality)
	assert.ElementsMatch(
		s.T(),
		highCardTags,
		[]string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh", "tag4:sourceClusterLow", "tag5:sourceLow"},
	)
}

type entityEventExpectation struct {
	eventType    types.EventType
	id           string
	lowCardTags  []string
	orchCardTags []string
	highCardTags []string
}

func TestSubscribe(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, nooptelemetry.Module())
	telemetryStore := taggerTelemetry.NewStore(tel)
	clock := clock.NewMock()
	store := newTagStoreWithClock(clock, telemetryStore)

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
