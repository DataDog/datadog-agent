// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package tagger

import (
	"math/rand"
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
	s.store = newTagStore()
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

	tagsHigh, sourcesHigh := s.store.lookup("test", true)
	tagsLow, sourcesLow := s.store.lookup("test", false)

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
	tags, sources := s.store.lookup("test", false)
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
	tagsHigh, sourcesHigh := s.store.lookup("test1", true)
	assert.Len(s.T(), tagsHigh, 3)
	assert.Len(s.T(), sourcesHigh, 2)
	tagsHigh, sourcesHigh = s.store.lookup("test2", true)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	s.store.prune()

	// deletion map should be empty now
	s.store.toDeleteMutex.RLock()
	assert.Len(s.T(), s.store.toDelete, 0)
	s.store.toDeleteMutex.RUnlock()

	// test1 should be removed, test2 still present
	tagsHigh, sourcesHigh = s.store.lookup("test1", true)
	assert.Nil(s.T(), tagsHigh)
	assert.Nil(s.T(), sourcesHigh)
	tagsHigh, sourcesHigh = s.store.lookup("test2", true)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

	err := s.store.prune()
	assert.Nil(s.T(), err)

	// No impact if nothing is queued
	tagsHigh, sourcesHigh = s.store.lookup("test1", true)
	assert.Nil(s.T(), tagsHigh)
	assert.Nil(s.T(), sourcesHigh)
	tagsHigh, sourcesHigh = s.store.lookup("test2", true)
	assert.Len(s.T(), tagsHigh, 2)
	assert.Len(s.T(), sourcesHigh, 1)

}

func TestStoreSuite(t *testing.T) {
	suite.Run(t, &StoreTestSuite{})
}

func TestGetEntityTags(t *testing.T) {
	etags := entityTags{
		lowCardTags:  make(map[string][]string),
		highCardTags: make(map[string][]string),
		cacheValid:   false,
	}
	assert.False(t, etags.cacheValid)

	// Get empty tags and make sure cache is now set to valid
	tags, sources := etags.get(true)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Add tags but don't invalidate the cache, we should return empty arrays
	etags.lowCardTags["source"] = []string{"low1", "low2"}
	etags.highCardTags["source"] = []string{"high1", "high2"}
	tags, sources = etags.get(true)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags, sources = etags.get(true)
	assert.Len(t, tags, 4)
	assert.ElementsMatch(t, tags, []string{"low1", "low2", "high1", "high2"})
	assert.Len(t, sources, 1)
	assert.True(t, etags.cacheValid)
	tags, sources = etags.get(false)
	assert.Len(t, tags, 2)
	assert.ElementsMatch(t, tags, []string{"low1", "low2"})
	assert.Len(t, sources, 1)
}

func TestDuplicateSourceTags(t *testing.T) {
	etags := entityTags{
		lowCardTags:  make(map[string][]string),
		highCardTags: make(map[string][]string),
		cacheValid:   false,
	}
	assert.False(t, etags.cacheValid)

	// Get empty tags and make sure cache is now set to valid
	tags, sources := etags.get(true)
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
	etags.lowCardTags["sourceNodeOrchestrator"] = []string{"bar", "tag1:sourceHigh", "tag2:sourceHigh"}
	etags.lowCardTags["sourceNodeRuntime"] = []string{"foo", "tag1:sourceLow", "tag2:sourceLow"}
	etags.highCardTags["sourceNodeRuntime"] = []string{"tag3:sourceLow", "tag5:sourceLow"}
	etags.highCardTags["sourceNodeOrchestrator"] = []string{"tag3:sourceHigh", "tag4:sourceHigh"}
	etags.highCardTags["sourceClusterOrchestrator"] = []string{"tag4:sourceClusterLow"}
	etags.lowCardTags["sourceClusterOrchestrator"] = []string{"tag3:sourceClusterHigh", "tag1:sourceClusterLow"}
	tags, sources = etags.get(true)
	assert.Len(t, tags, 0)
	assert.Len(t, sources, 0)
	assert.True(t, etags.cacheValid)

	// Invalidate the cache, we should now get the tags
	etags.cacheValid = false
	tags, sources = etags.get(true)
	assert.Len(t, tags, 7)
	assert.ElementsMatch(t, tags, []string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh", "tag4:sourceClusterLow", "tag5:sourceLow"})
	assert.Len(t, sources, 3)
	assert.True(t, etags.cacheValid)
	tags, sources = etags.get(false)
	assert.Len(t, sources, 3)
	assert.Len(t, tags, 5)
	assert.ElementsMatch(t, tags, []string{"foo", "bar", "tag1:sourceClusterLow", "tag2:sourceHigh", "tag3:sourceClusterHigh"})
}

func shuffleTags(tags []string) {
	for i := range tags {
		j := rand.Intn(i + 1)
		tags[i], tags[j] = tags[j], tags[i]
	}
}

func TestDigest(t *testing.T) {
	for _, tc := range []struct {
		tagInfo *collectors.TagInfo
	}{
		{
			tagInfo: &collectors.TagInfo{
				Source: "source-a",
				Entity: "docker://4bf586f5309008b9822d3a7aa819cb123b23959f6ce63446dd2a97523b531dfe",
				HighCardTags: []string{
					"high2:b",
					"high1:a",
					"high3:c",
				},
				LowCardTags: []string{
					"low2:b",
					"low1:a",
					"low3:c",
				},
			},
		},
		{
			tagInfo: &collectors.TagInfo{
				Source:       "source-b",
				Entity:       "docker://4bf586f5309008b9822d3a7aa819cb123b23959f6ce63446dd2a97523b531dfe",
				HighCardTags: []string{},
				LowCardTags: []string{
					"low3:c",
				},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			for i := 0; i < 100; i++ {
				beforeShuffle := computeTagsHash(tc.tagInfo)
				shuffleTags(tc.tagInfo.LowCardTags)
				shuffleTags(tc.tagInfo.HighCardTags)
				assert.Equal(t, beforeShuffle, computeTagsHash(tc.tagInfo))
			}
		})
	}
}
