// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package tagger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
)

type StoreTestSuite struct {
	suite.Suite
	store *tagStore
}

func (s *StoreTestSuite) SetupTest() {
	var err error
	s.store, err = newTagStore()
	assert.Nil(s.T(), err)
}

func (s *StoreTestSuite) TestIngest() {
	s.store.processTagInfo(&collectors.TagInfo{
		Source:       "source1",
		Entity:       "test",
		LowCardTags:  []string{"tag"},
		HighCardTags: []string{"tag"},
	})
	s.store.processTagInfo(&collectors.TagInfo{
		Source:      "source2",
		Entity:      "test",
		LowCardTags: []string{"tag"},
	})

	s.store.storeMutex.RLock()
	defer s.store.storeMutex.RUnlock()

	assert.Len(s.T(), s.store.store, 1)
	assert.Len(s.T(), s.store.store["test"].lowCardTags, 2)
	assert.Len(s.T(), s.store.store["test"].highCardTags, 2)
}

func (s *StoreTestSuite) TestLookup() {
	s.store.processTagInfo(&collectors.TagInfo{
		Source:       "source1",
		Entity:       "test",
		LowCardTags:  []string{"tag"},
		HighCardTags: []string{"tag"},
	})
	s.store.processTagInfo(&collectors.TagInfo{
		Source:      "source2",
		Entity:      "test",
		LowCardTags: []string{"tag"},
	})

	tagsHigh, sourcesHigh, err := s.store.lookup("test", true)
	assert.Nil(s.T(), err)
	tagsLow, sourcesLow, err := s.store.lookup("test", false)
	assert.Nil(s.T(), err)

	assert.Len(s.T(), tagsHigh, 3)
	assert.Len(s.T(), tagsLow, 2)

	assert.Len(s.T(), sourcesHigh, 2)
	assert.Contains(s.T(), sourcesHigh, "source1")
	assert.Contains(s.T(), sourcesHigh, "source2")

	assert.Len(s.T(), sourcesLow, 2)
	assert.Contains(s.T(), sourcesLow, "source1")
	assert.Contains(s.T(), sourcesHigh, "source2")
}

func (s *StoreTestSuite) TestLookupNotPresent() {
	tags, sources, err := s.store.lookup("test", false)
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), tags)
	assert.Nil(s.T(), sources)
}

func (s *StoreTestSuite) TestPrune() {
	s.store.toDeleteMutex.RLock()
	assert.Len(s.T(), s.store.toDelete, 0)
	s.store.toDeleteMutex.RUnlock()

	// Adds
	s.store.processTagInfo(&collectors.TagInfo{
		Source:       "source1",
		Entity:       "test1",
		LowCardTags:  []string{"tag"},
		HighCardTags: []string{"tag"},
	})
	s.store.processTagInfo(&collectors.TagInfo{
		Source:      "source2",
		Entity:      "test1",
		LowCardTags: []string{"tag"},
	})
	s.store.processTagInfo(&collectors.TagInfo{
		Source:       "source1",
		Entity:       "test2",
		LowCardTags:  []string{"tag"},
		HighCardTags: []string{"tag"},
	})

	// Deletion, to be batched
	s.store.processTagInfo(&collectors.TagInfo{
		Source:       "source1",
		Entity:       "test1",
		DeleteEntity: true,
	})

	s.store.toDeleteMutex.RLock()
	assert.Len(s.T(), s.store.toDelete, 1)
	s.store.toDeleteMutex.RUnlock()

	// Data should still be in the store
	tagsHigh, sourcesHigh, err := s.store.lookup("test1", true)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), tagsHigh, 3)
	assert.Len(s.T(), sourcesHigh, 2)
	tagsHigh, sourcesHigh, err = s.store.lookup("test2", true)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	s.store.prune()

	// deletion map should be empty now
	s.store.toDeleteMutex.RLock()
	assert.Len(s.T(), s.store.toDelete, 0)
	s.store.toDeleteMutex.RUnlock()

	// test1 should be removed, test2 still present
	tagsHigh, sourcesHigh, err = s.store.lookup("test1", true)
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), tagsHigh)
	assert.Nil(s.T(), sourcesHigh)
	tagsHigh, sourcesHigh, err = s.store.lookup("test2", true)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	err = s.store.prune()
	assert.Nil(s.T(), err)

	// No impact if nothing is queued
	tagsHigh, sourcesHigh, err = s.store.lookup("test1", true)
	assert.Nil(s.T(), err)
	assert.Nil(s.T(), tagsHigh)
	assert.Nil(s.T(), sourcesHigh)
	tagsHigh, sourcesHigh, err = s.store.lookup("test2", true)
	assert.Nil(s.T(), err)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}
