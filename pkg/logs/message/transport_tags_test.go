// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type fakeTagFilter struct {
	drop map[string]bool
}

func (f *fakeTagFilter) Keep(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		key, _, _ := strings.Cut(tag, ":")
		if !f.drop[key] {
			out = append(out, tag)
		}
	}
	return out
}

func (f *fakeTagFilter) RetainsTag(key, _ string) bool {
	return !f.drop[key]
}

func TestTransportTagsFiltersWithoutChangingStoredTags(t *testing.T) {
	cfg := &config.LogsConfig{
		SourceCategory: "cat",
		Tags:           []string{"cfg:tag", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})

	filtered := origin.TransportTags(&fakeTagFilter{drop: map[string]bool{"foo": true}})

	assert.Equal(t, []string{"baz", "sourcecategory:cat", "cfg:tag", "e"}, filtered)
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:cat", "cfg:tag", "e"}, origin.Tags())
}

func TestTransportTagsDoesNotAliasInputs(t *testing.T) {
	for _, filter := range []*fakeTagFilter{
		{drop: map[string]bool{}},
		{drop: map[string]bool{"foo": true}},
	} {
		cfg := &config.LogsConfig{Tags: []string{"cfg:tag"}}
		origin := NewOrigin(sources.NewLogSource("", cfg))

		tagsWithCapacity := make([]string, 2, 10)
		tagsWithCapacity[0] = "foo:bar"
		tagsWithCapacity[1] = "baz"
		origin.SetTags(tagsWithCapacity)

		result := origin.TransportTags(filter)
		if len(result) > 0 {
			result[0] = "mutated:value"
		}

		assert.Equal(t, []string{"foo:bar", "baz"}, origin.tags)
		assert.Equal(t, []string{"cfg:tag"}, []string(cfg.Tags))
	}
}
