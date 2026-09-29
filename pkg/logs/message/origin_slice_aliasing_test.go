// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestTags_NoMutation verifies that Tags() does not mutate the origin's tags slice.
//
// Accidental mutation may happen if for instance `tags := o.tags; append(tags,
// ...)` is used, creating an alias that shares the backing array. If there's spare
// capacity in `o.tags` then `append()` modifies the shared backing array.
//
// The consequence of this is a mistagging of logs.
func TestTags_NoMutation(t *testing.T) {
	cfg := &config.LogsConfig{}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)

	// Set tags with extra capacity to expose aliasing bugs
	tagsWithCapacity := make([]string, 2, 10)
	tagsWithCapacity[0] = "tag1:value1"
	tagsWithCapacity[1] = "tag2:value2"
	origin.SetTags(tagsWithCapacity)

	// Capture original state
	originalTags := origin.tags
	originalLen := len(originalTags)

	result := origin.Tags()

	// Verify: Result contains all expected tags
	assert.Contains(t, result, "tag1:value1")
	assert.Contains(t, result, "tag2:value2")

	// Verify: Original wasn't mutated
	assert.Len(t, originalTags, originalLen, "origin.tags should not be modified")
	assert.Equal(t, "tag1:value1", origin.tags[0])
	assert.Equal(t, "tag2:value2", origin.tags[1])
}

// TestTags_NoMutationWithSnapshot is the snapshot variant of
// TestTags_NoMutation: building and reading the snapshot must not write into
// the origin's tags slice, even when it has spare capacity.
func TestTags_NoMutationWithSnapshot(t *testing.T) {
	cfg := &config.LogsConfig{SourceCategory: "web", Tags: []string{"env:prod"}}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)

	tagsWithCapacity := make([]string, 2, 10)
	tagsWithCapacity[0] = "tag1:value1"
	tagsWithCapacity[1] = "tag2:value2"
	origin.SetTags(tagsWithCapacity)
	origin.BuildTagSnapshot()

	result := origin.Tags()
	_ = origin.TagsPayload([]string{"processing:tag"})

	assert.Equal(t, []string{"tag1:value1", "tag2:value2", "sourcecategory:web", "env:prod"}, result)
	assert.Len(t, origin.tags, 2, "origin.tags should not be modified")
	assert.Equal(t, "tag1:value1", origin.tags[0])
	assert.Equal(t, "tag2:value2", origin.tags[1])
	// Nothing was written into the spare capacity.
	assert.Equal(t, "", tagsWithCapacity[:3][2])
}
