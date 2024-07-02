// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

const (
	testEntityID  = "testEntityID"
	testSource    = "testSource"
	invalidSource = "invalidSource"
)

func TestToEntity(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
		orchestratorCardTags: []string{"o1:v1", "o2:v2"},
		highCardTags:         []string{"h1:v1", "h2:v2"},
		standardTags:         []string{"service:s1"},
	})

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags: []string{"l3:v3"},
	})

	assert.Equal(
		t,
		types.Entity{
			ID:                          testEntityID,
			LowCardinalityTags:          []string{"l1:v1", "l2:v2", "service:s1"},
			OrchestratorCardinalityTags: []string{"o1:v1", "o2:v2"},
			HighCardinalityTags:         []string{"h1:v1", "h2:v2"},
			StandardTags:                []string{"service:s1"},
		},
		entityTags.toEntity(),
	)
}

func TestGetStandard(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
		orchestratorCardTags: []string{"o1:v1", "o2:v2"},
		highCardTags:         []string{"h1:v1", "h2:v2"},
		standardTags:         []string{"service:s1"},
	})

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags:  []string{"l3:v3", "service:s2"},
		standardTags: []string{"service:s2"},
	})

	assert.Equal(t, []string{"service:s1"}, entityTags.getStandard())
}

func TestGetHashedTags(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
		orchestratorCardTags: []string{"o1:v1", "o2:v2"},
		highCardTags:         []string{"h1:v1", "h2:v2"},
		standardTags:         []string{"service:s1"},
	})

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags: []string{"l3:v3"},
	})

	assert.ElementsMatch(
		t,
		[]string{"l1:v1", "l2:v2", "service:s1"},
		entityTags.getHashedTags(types.LowCardinality).Get(),
	)

	assert.ElementsMatch(
		t,
		[]string{"l1:v1", "l2:v2", "service:s1", "o1:v1", "o2:v2"},
		entityTags.getHashedTags(types.OrchestratorCardinality).Get(),
	)

	assert.ElementsMatch(
		t,
		[]string{"l1:v1", "l2:v2", "service:s1", "o1:v1", "o2:v2", "h1:v1", "h2:v2"},
		entityTags.getHashedTags(types.HighCardinality).Get(),
	)
}

func TestTagsForSource(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags:          []string{"l1:v1", "l2:v2", "service:s1"},
		orchestratorCardTags: []string{"o1:v1", "o2:v2"},
		highCardTags:         []string{"h1:v1", "h2:v2"},
		standardTags:         []string{"service:s1"},
	})

	// Compare the tags (the order does not matter)
	tags := entityTags.tagsForSource(testSource)
	assert.ElementsMatch(t, []string{"l1:v1", "l2:v2", "service:s1"}, tags.lowCardTags)
	assert.ElementsMatch(t, []string{"o1:v1", "o2:v2"}, tags.orchestratorCardTags)
	assert.ElementsMatch(t, []string{"h1:v1", "h2:v2"}, tags.highCardTags)
	assert.ElementsMatch(t, []string{"service:s1"}, tags.standardTags)

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags: []string{"l3:v3"},
	})
	assert.Nil(t, entityTags.tagsForSource(invalidSource))
}

func TestTagsBySource(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags:          []string{"l1:v1", "l2:v2"},
		orchestratorCardTags: []string{"o1:v1", "o2:v2"},
		highCardTags:         []string{"h1:v1", "h2:v2"},
	})

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags: []string{"l3:v3"},
	})

	assert.Equal(
		t,
		map[string][]string{
			testSource: {"l1:v1", "l2:v2", "o1:v1", "o2:v2", "h1:v1", "h2:v2"},
		},
		entityTags.tagsBySource(),
	)
}

func TestSources(t *testing.T) {
	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags: []string{"l1:v1"},
	})

	// Different source is ignored
	entityTags.setTagsForSource(invalidSource, sourceTags{
		lowCardTags: []string{"l2:v2"},
	})

	assert.ElementsMatch(t, []string{testSource}, entityTags.sources())
}

func TestDeleteSource(t *testing.T) {
	expiryDate := time.Now().Add(time.Hour)

	entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

	entityTags.setTagsForSource(testSource, sourceTags{
		lowCardTags: []string{"l1:v1"},
	})

	entityTags.setSourceExpiration(testSource, expiryDate)

	// Different source is ignored
	entityTags.setSourceExpiration(invalidSource, time.Now())

	assert.Equal(t, expiryDate, entityTags.expiryDate)
}

func TestDeleteExpired(t *testing.T) {
	expiryDate := time.Now()

	tests := []struct {
		name               string
		time               time.Time
		expectShouldRemove bool
	}{
		{
			name:               "expired",
			time:               expiryDate.Add(time.Minute),
			expectShouldRemove: true,
		},
		{
			name:               "not expired",
			time:               expiryDate.Add(-time.Minute),
			expectShouldRemove: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entityTags := newEntityTagsWithSingleSource(testEntityID, testSource)

			// Entity tags without tags are marked for deletion so set some to
			// make sure it's not removed for that reason
			entityTags.setTagsForSource(testSource, sourceTags{
				lowCardTags: []string{"l1:v1"},
			})

			entityTags.setSourceExpiration(testSource, expiryDate)
			entityTags.deleteExpired(test.time)
			assert.Equal(t, test.expectShouldRemove, entityTags.shouldRemove())
		})
	}
}
