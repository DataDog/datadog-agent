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
	source.SetTagFilterIfUnset(&fakeTagFilter{drop: map[string]bool{"foo": true}})

	filtered := origin.TransportTags()

	assert.Equal(t, []string{"baz", "sourcecategory:cat", "cfg:tag", "e"}, filtered)
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:cat", "cfg:tag", "e"}, origin.Tags())
}
