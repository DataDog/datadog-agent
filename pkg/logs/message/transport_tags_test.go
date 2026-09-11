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

// fakeTagFilter drops any tag whose key is in Drop, and records every
// argument it is called with. It intentionally does not depend on the real
// matcher, which is being implemented concurrently.
type fakeTagFilter struct {
	Drop         map[string]bool
	KeepCalls    [][]string
	RetainsCalls []string
}

func (f *fakeTagFilter) Keep(tags []string) []string {
	f.KeepCalls = append(f.KeepCalls, tags)
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		key, _, _ := strings.Cut(t, ":")
		if f.Drop[key] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (f *fakeTagFilter) Retains(tag string) bool {
	f.RetainsCalls = append(f.RetainsCalls, tag)
	key, _, _ := strings.Cut(tag, ":")
	return !f.Drop[key]
}

func newTransportTagsTestOrigin() *Origin {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "cat",
		Tags:           []string{"cfg:tag", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})
	return origin
}

func TestTransportTags_UnfilteredTagsUntouched(t *testing.T) {
	origin := newTransportTagsTestOrigin()
	f := &fakeTagFilter{Drop: map[string]bool{"foo": true}}

	all := origin.Tags()
	filtered := origin.TransportTags(f)

	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:cat", "cfg:tag", "e"}, all)
	assert.Equal(t, []string{"baz", "sourcecategory:cat", "cfg:tag", "e"}, filtered)
}

func TestTransportTags_NilFilterMatchesTags(t *testing.T) {
	origin := newTransportTagsTestOrigin()
	assert.Equal(t, origin.Tags(), origin.TransportTags(nil))
}

func TestTransportTags_PreservesOrder(t *testing.T) {
	origin := newTransportTagsTestOrigin()

	// A filter that drops nothing must not reorder anything.
	assert.Equal(t, origin.Tags(), origin.TransportTags(&fakeTagFilter{Drop: map[string]bool{}}))

	// Dropping one group's tag must not disturb the relative order of the rest.
	f := &fakeTagFilter{Drop: map[string]bool{"cfg": true}}
	assert.Equal(t, []string{"foo:bar", "baz", "sourcecategory:cat", "e"}, origin.TransportTags(f))
}

func TestTransportTags_NoMutationNoAliasing(t *testing.T) {
	// One filter that drops nothing, one that drops something.
	filters := []*fakeTagFilter{
		{Drop: map[string]bool{}},
		{Drop: map[string]bool{"foo": true}},
	}
	for _, f := range filters {
		cfg := &config.LogsConfig{Source: "a", Tags: []string{"cfg:tag"}}
		source := sources.NewLogSource("", cfg)
		origin := NewOrigin(source)

		// Extra capacity exposes aliasing bugs: append() on a slice with spare
		// capacity writes into the shared backing array.
		tagsWithCapacity := make([]string, 2, 10)
		tagsWithCapacity[0] = "foo:bar"
		tagsWithCapacity[1] = "baz"
		origin.SetTags(tagsWithCapacity)

		result := origin.TransportTags(f)
		if len(result) > 0 {
			result[0] = "mutated:value"
		}

		assert.Equal(t, "foo:bar", origin.tags[0])
		assert.Equal(t, "baz", origin.tags[1])
		assert.Equal(t, []string{"cfg:tag"}, []string(cfg.Tags))
	}
}

func TestTransportTagsPayload_DdsourceNeverFiltered(t *testing.T) {
	cfg := &config.LogsConfig{Source: "mysource"}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)

	f := &fakeTagFilter{Drop: map[string]bool{"source": true}}
	payload := string(origin.TransportTagsPayload(f, nil))

	assert.Contains(t, payload, "[dd ddsource=\"mysource\"]")
}

func TestTransportTagsPayload_SourceCategoryDroppedViaRetains(t *testing.T) {
	cfg := &config.LogsConfig{Source: "a", SourceCategory: "cat"}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)

	f := &fakeTagFilter{Drop: map[string]bool{"sourcecategory": true}}
	payload := string(origin.TransportTagsPayload(f, nil))

	assert.NotContains(t, payload, "ddsourcecategory")
	assert.Contains(t, f.RetainsCalls, "sourcecategory:cat")
}

func TestTransportTagsPayload_DdtagsFiltered(t *testing.T) {
	cfg := &config.LogsConfig{Source: "a", Tags: []string{"keep:me", "drop:me"}}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)

	f := &fakeTagFilter{Drop: map[string]bool{"drop": true}}
	payload := string(origin.TransportTagsPayload(f, nil))

	assert.Contains(t, payload, "ddtags=\"keep:me\"")
	assert.NotContains(t, payload, "drop:me")
}

func TestTransportTagsPayload_NilFilterMatchesTagsPayload(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"c:d", "e"},
	}
	source := sources.NewLogSource("", cfg)
	origin := NewOrigin(source)
	origin.SetTags([]string{"foo:bar", "baz"})

	processingTags := []string{"processing:tag"}
	assert.Equal(t, origin.TagsPayload(processingTags), origin.TransportTagsPayload(nil, processingTags))
}
