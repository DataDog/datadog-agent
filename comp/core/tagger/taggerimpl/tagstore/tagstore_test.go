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
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type StoreTestSuite struct {
	suite.Suite
	clock    *clock.Mock
	tagstore *TagStore
}

func (s *StoreTestSuite) SetupTest() {
	tel := fxutil.Test[telemetry.Component](s.T(), telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	s.clock = clock.NewMock()
	// set the mock clock to the current time
	s.clock.Add(time.Since(time.Unix(0, 0)))

	mockConfig := configmock.New(s.T())
	s.tagstore = newTagStoreWithClock(mockConfig, s.clock, telemetryStore)
}

func (s *StoreTestSuite) TestIngest() {
	entityID := types.NewEntityID(types.ContainerID, "test")

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:               "source1",
			EntityID:             entityID,
			LowCardTags:          []string{"tag"},
			OrchestratorCardTags: []string{"tag"},
			HighCardTags:         []string{"tag"},
		},
		{
			Source:      "source2",
			EntityID:    entityID,
			LowCardTags: []string{"tag"},
		},
	})

	assert.Equalf(s.T(), s.tagstore.store.Size(), 1, "expected tagstore to contain 1 TagEntity, but found: s.tagstore.store.size()")

	storedTags, exists := s.tagstore.store.Get(entityID)
	require.True(s.T(), exists)
	assert.Len(s.T(), storedTags.sources(), 2)
}

func (s *StoreTestSuite) TestLookup() {
	entityID := types.NewEntityID(types.ContainerID, "test")
	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source1",
			EntityID:     entityID,
			LowCardTags:  []string{"tag"},
			HighCardTags: []string{"tag"},
		},
		{
			Source:      "source2",
			EntityID:    entityID,
			LowCardTags: []string{"tag"},
		},
		{
			Source:               "source3",
			EntityID:             entityID,
			OrchestratorCardTags: []string{"tag"},
		},
	})

	tagsHigh := s.tagstore.Lookup(entityID, types.HighCardinality)
	tagsOrch := s.tagstore.Lookup(entityID, types.OrchestratorCardinality)
	tagsLow := s.tagstore.Lookup(entityID, types.LowCardinality)

	assert.Len(s.T(), tagsHigh, 4)
	assert.Len(s.T(), tagsLow, 2)
	assert.Len(s.T(), tagsOrch, 3)
}

func (s *StoreTestSuite) TestLookupHashedWithEntityStr() {
	entityID := types.NewEntityID(types.ContainerID, "test")
	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source1",
			EntityID:     entityID,
			LowCardTags:  []string{"low1"},
			HighCardTags: []string{"high1"},
		},
		{
			Source:      "source2",
			EntityID:    entityID,
			LowCardTags: []string{"low2"},
		},
		{
			Source:               "source3",
			EntityID:             entityID,
			OrchestratorCardTags: []string{"orchestrator1"},
		},
	})

	tagsLow := s.tagstore.LookupHashedWithEntityStr(entityID, types.LowCardinality)
	tagsOrch := s.tagstore.LookupHashedWithEntityStr(entityID, types.OrchestratorCardinality)
	tagsHigh := s.tagstore.LookupHashedWithEntityStr(entityID, types.HighCardinality)

	assert.ElementsMatch(s.T(), tagsLow.Get(), []string{"low1", "low2"})
	assert.ElementsMatch(s.T(), tagsOrch.Get(), []string{"low1", "low2", "orchestrator1"})
	assert.ElementsMatch(s.T(), tagsHigh.Get(), []string{"low1", "low2", "orchestrator1", "high1"})
}

func (s *StoreTestSuite) TestLookupStandard() {
	entityID := types.NewEntityID(types.ContainerID, "test")

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source1",
			EntityID:     entityID,
			LowCardTags:  []string{"tag", "env:dev"},
			StandardTags: []string{"env:dev"},
		},
		{
			Source:       "source2",
			EntityID:     entityID,
			LowCardTags:  []string{"tag", "service:foo"},
			StandardTags: []string{"service:foo"},
		},
	})

	standard, err := s.tagstore.LookupStandard(entityID)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), standard, 2)
	assert.Contains(s.T(), standard, "env:dev")
	assert.Contains(s.T(), standard, "service:foo")

	_, err = s.tagstore.LookupStandard(types.NewEntityID("not", "found"))
	assert.NotNil(s.T(), err)
}

func (s *StoreTestSuite) TestLookupNotPresent() {
	entityID := types.NewEntityID(types.ContainerID, "test")
	tags := s.tagstore.Lookup(entityID, types.LowCardinality)
	assert.Nil(s.T(), tags)
}

func (s *StoreTestSuite) TestPrune__deletedEntities() {
	entityID1 := types.NewEntityID(types.ContainerID, "test1")
	entityID2 := types.NewEntityID(types.ContainerID, "test2")

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		// Adds
		{
			Source:               "source1",
			EntityID:             entityID1,
			LowCardTags:          []string{"s1tag"},
			OrchestratorCardTags: []string{"s1tag"},
			HighCardTags:         []string{"s1tag"},
		},
		{
			Source:       "source2",
			EntityID:     entityID1,
			HighCardTags: []string{"s2tag"},
		},
		{
			Source:       "source1",
			EntityID:     entityID2,
			LowCardTags:  []string{"tag"},
			HighCardTags: []string{"tag"},
		},

		// Deletion, to be batched
		{
			Source:       "source1",
			EntityID:     entityID1,
			DeleteEntity: true,
		},
	})

	// Data should still be in the store
	tagsHigh := s.tagstore.Lookup(entityID1, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 4)
	tagsOrch := s.tagstore.Lookup(entityID1, types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	tagsHigh = s.tagstore.Lookup(entityID2, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)

	s.clock.Add(10 * time.Minute)
	s.tagstore.Prune()

	// test1 should only have tags from source2, source1 should be removed
	tagsHigh = s.tagstore.Lookup(entityID1, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsOrch = s.tagstore.Lookup(entityID1, types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 0)

	// test2 should still be present
	tagsHigh = s.tagstore.Lookup(entityID2, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		// re-add tags from removed source, then remove another one
		{
			Source:      "source1",
			EntityID:    entityID1,
			LowCardTags: []string{"s1tag"},
		},
		// Deletion, to be batched
		{
			Source:       "source2",
			EntityID:     entityID1,
			DeleteEntity: true,
		},
	})

	s.clock.Add(10 * time.Minute)
	s.tagstore.Prune()

	tagsHigh = s.tagstore.Lookup(entityID1, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsHigh = s.tagstore.Lookup(entityID2, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 2)
}

func (s *StoreTestSuite) TestPrune__emptyEntries() {
	entityID1 := types.NewEntityID(types.ContainerID, "test1")
	entityID2 := types.NewEntityID(types.ContainerID, "test2")
	entityID3 := types.NewEntityID(types.ContainerID, "test3")
	emptyEntityID1 := types.NewEntityID(types.ContainerID, "emptyEntity1")
	emptyEntityID2 := types.NewEntityID(types.ContainerID, "emptyEntity2")

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:               "source1",
			EntityID:             entityID1,
			LowCardTags:          []string{"s1tag"},
			OrchestratorCardTags: []string{"s1tag"},
			HighCardTags:         []string{"s1tag"},
		},
		{
			Source:       "source2",
			EntityID:     entityID2,
			HighCardTags: []string{"s2tag"},
		},
		{
			Source:      "emptySource1",
			EntityID:    emptyEntityID1,
			LowCardTags: []string{},
		},
		{
			Source:       "emptySource2",
			EntityID:     emptyEntityID2,
			StandardTags: []string{},
		},
		{
			Source:      "emptySource3",
			EntityID:    entityID3,
			LowCardTags: []string{},
		},
		{
			Source:      "source3",
			EntityID:    entityID3,
			LowCardTags: []string{"s3tag"},
		},
	})

	tagStoreSize := s.tagstore.store.Size()
	assert.Equalf(s.T(), tagStoreSize, 5, "should have 5 item(s), but has %d", tagStoreSize)

	s.tagstore.Prune()

	tagStoreSize = s.tagstore.store.Size()
	assert.Equalf(s.T(), tagStoreSize, 3, "should have 3 item(s), but has %d", tagStoreSize)

	// Assert non-empty tags aren't deleted
	tagsHigh := s.tagstore.Lookup(entityID1, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 3)
	tagsOrch := s.tagstore.Lookup(entityID1, types.OrchestratorCardinality)
	assert.Len(s.T(), tagsOrch, 2)
	tagsHigh = s.tagstore.Lookup(entityID2, types.HighCardinality)
	assert.Len(s.T(), tagsHigh, 1)
	tagsLow := s.tagstore.Lookup(entityID3, types.LowCardinality)
	assert.Len(s.T(), tagsLow, 1)

	// Assert empty entities are deleted
	emptyTags1 := s.tagstore.Lookup(emptyEntityID1, types.HighCardinality)
	assert.Len(s.T(), emptyTags1, 0)
	emptyTags2 := s.tagstore.Lookup(emptyEntityID2, types.HighCardinality)
	assert.Len(s.T(), emptyTags2, 0)
}

func (s *StoreTestSuite) TestList() {
	entityID1 := types.NewEntityID(types.ContainerID, "entity-1")
	entityID2 := types.NewEntityID(types.ContainerID, "entity-2")

	s.tagstore.ProcessTagInfo(
		[]*types.TagInfo{
			{
				Source:               "source-1",
				EntityID:             entityID1,
				HighCardTags:         []string{"h1:v1", "h2:v2"},
				OrchestratorCardTags: []string{"o1:v1", "o2:v2"},
				LowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
			{
				Source:               "source-1",
				EntityID:             entityID2,
				HighCardTags:         []string{"h3:v3", "h4:v4"},
				OrchestratorCardTags: []string{"o3:v3", "o4:v4"},
				LowCardTags:          []string{"l3:v3", "l4:v4", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
		},
	)

	resultList := s.tagstore.List()
	require.Equal(s.T(), 2, len(resultList.Entities))

	entity1, ok := resultList.Entities[entityID1.String()]
	require.True(s.T(), ok)
	require.Equal(s.T(), 1, len(entity1.Tags))
	require.ElementsMatch( // Tags order is not important
		s.T(),
		entity1.Tags["source-1"],
		[]string{"l1:v1", "l2:v2", "service:s1", "o1:v1", "o2:v2", "h1:v1", "h2:v2"},
	)

	entity2, ok := resultList.Entities[entityID2.String()]
	require.True(s.T(), ok)
	require.Equal(s.T(), 1, len(entity2.Tags))
	require.ElementsMatch( // Tags order is not important
		s.T(),
		entity2.Tags["source-1"],
		[]string{"l3:v3", "l4:v4", "service:s1", "o3:v3", "o4:v4", "h3:v3", "h4:v4"},
	)
}

func (s *StoreTestSuite) TestGetEntity() {
	entityID1 := types.NewEntityID(types.ContainerID, "entity-1")
	_, err := s.tagstore.GetEntity(entityID1)
	require.Error(s.T(), err)

	s.tagstore.ProcessTagInfo(
		[]*types.TagInfo{
			{
				Source:               "source-1",
				EntityID:             entityID1,
				HighCardTags:         []string{"h1:v1", "h2:v2"},
				OrchestratorCardTags: []string{"o1:v1", "o2:v2"},
				LowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
				StandardTags:         []string{"service:s1"},
			},
		},
	)

	entity, err := s.tagstore.GetEntity(entityID1)
	require.NoError(s.T(), err)
	assert.Equal(
		s.T(),
		&types.Entity{
			ID:                          entityID1,
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
	entityIDA := types.NewEntityID(types.ContainerID, "entityA")
	entityIDB := types.NewEntityID(types.ContainerID, "entityB")
	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			EntityID:     entityIDA,
			HighCardTags: []string{"expired"},
			ExpiryDate:   s.clock.Now().Add(-10 * time.Second),
		},
		{
			Source:       "source",
			EntityID:     entityIDB,
			HighCardTags: []string{"expiresSoon"},
			ExpiryDate:   s.clock.Now().Add(10 * time.Second),
		},
	})

	s.tagstore.Prune()

	tagsHigh := s.tagstore.Lookup(entityIDB, types.HighCardinality)
	assert.Contains(s.T(), tagsHigh, "expiresSoon")

	tagsHigh = s.tagstore.Lookup(entityIDA, types.HighCardinality)
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

	testEntityID := types.NewEntityID(types.ContainerID, "testEntityID")

	// Mock collector priorities
	collectors.CollectorPriorities = map[string]types.CollectorPriority{
		"sourceNodeRuntime":         types.NodeRuntime,
		"sourceNodeOrchestrator":    types.NodeOrchestrator,
		"sourceClusterOrchestrator": types.ClusterOrchestrator,
	}

	nodeRuntimeTags := types.TagInfo{
		Source:       "sourceNodeRuntime",
		EntityID:     testEntityID,
		LowCardTags:  []string{"foo", "tag1:sourceLow", "tag2:sourceLow"},
		HighCardTags: []string{"tag3:sourceLow", "tag5:sourceLow"},
	}

	nodeOrchestractorTags := types.TagInfo{
		Source:       "sourceNodeOrchestrator",
		EntityID:     testEntityID,
		LowCardTags:  []string{"bar", "tag1:sourceHigh", "tag2:sourceHigh"},
		HighCardTags: []string{"tag3:sourceHigh", "tag4:sourceHigh"},
	}

	clusterOrchestratorTags := types.TagInfo{
		Source:       "sourceClusterOrchestrator",
		EntityID:     testEntityID,
		LowCardTags:  []string{"tag1:sourceClusterLow", "tag3:sourceClusterHigh"},
		HighCardTags: []string{"tag4:sourceClusterLow"},
	}

	s.tagstore.ProcessTagInfo([]*types.TagInfo{
		&nodeRuntimeTags,
		&nodeOrchestractorTags,
		&clusterOrchestratorTags,
	})

	lowCardTags := s.tagstore.Lookup(testEntityID, types.LowCardinality)
	assert.ElementsMatch(
		s.T(),
		lowCardTags,
		[]string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh"},
	)

	highCardTags := s.tagstore.Lookup(testEntityID, types.HighCardinality)
	assert.ElementsMatch(
		s.T(),
		highCardTags,
		[]string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh", "tag4:sourceClusterLow", "tag5:sourceLow"},
	)
}

type entityEventExpectation struct {
	eventType    types.EventType
	id           types.EntityID
	lowCardTags  []string
	orchCardTags []string
	highCardTags []string
}

func TestSubscribe(t *testing.T) {
	tel := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	telemetryStore := taggerTelemetry.NewStore(tel)
	clock := clock.NewMock()
	mockConfig := configmock.New(t)
	store := newTagStoreWithClock(mockConfig, clock, telemetryStore)

	collectors.CollectorPriorities["source2"] = types.ClusterOrchestrator
	collectors.CollectorPriorities["source"] = types.NodeRuntime

	entityID1 := types.NewEntityID(types.ContainerID, "test1")
	entityID2 := types.NewEntityID(types.ContainerID, "test2")
	var expectedEvents = []entityEventExpectation{
		{types.EventTypeAdded, entityID1, []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, entityID1, []string{"low"}, []string{"orch"}, []string{"high:1", "high:2"}},
		{types.EventTypeAdded, entityID2, []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeModified, entityID1, []string{"low"}, []string{}, []string{"high"}},
		{types.EventTypeDeleted, entityID1, nil, nil, nil},
	}

	store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			EntityID:     entityID1,
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	highCardEvents := []types.EntityEvent{}
	lowCardEvents := []types.EntityEvent{}

	highCardSubID := "high-card-sub-id"
	highCardSubscription, err := store.Subscribe(highCardSubID, types.NewFilterBuilder().Build(types.HighCardinality))
	require.NoError(t, err)

	lowCardSubID := "low-card-sub-id"
	lowCardSubscription, err := store.Subscribe(lowCardSubID, types.NewFilterBuilder().Build(types.LowCardinality))
	require.NoError(t, err)

	store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:               "source2",
			EntityID:             entityID1,
			LowCardTags:          []string{"low"},
			OrchestratorCardTags: []string{"orch"},
			HighCardTags:         []string{"high:1", "high:2"},
		},
		{
			Source:       "source2",
			EntityID:     entityID1,
			DeleteEntity: true,
		},
		{
			Source:       "source",
			EntityID:     entityID2,
			LowCardTags:  []string{"low"},
			HighCardTags: []string{"high"},
		},
	})

	clock.Add(10 * time.Minute)
	store.Prune()

	store.ProcessTagInfo([]*types.TagInfo{
		{
			Source:       "source",
			EntityID:     entityID1,
			DeleteEntity: true,
		},
	})

	clock.Add(10 * time.Minute)
	store.Prune()

	var wg sync.WaitGroup
	wg.Add(2)

	go collectEvents(&wg, &highCardEvents, highCardSubscription.EventsChan())
	go collectEvents(&wg, &lowCardEvents, lowCardSubscription.EventsChan())

	highCardSubscription.Unsubscribe()
	lowCardSubscription.Unsubscribe()
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
