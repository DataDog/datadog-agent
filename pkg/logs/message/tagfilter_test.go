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

type excludeKeysFilter struct {
	keys []string
	seen [][]string
}

func excludeKeys(keys ...string) *excludeKeysFilter {
	return &excludeKeysFilter{keys: keys}
}

func (f *excludeKeysFilter) Apply(tags []string) []string {
	f.seen = append(f.seen, tags)
	kept := make([]string, 0, len(tags))
	for _, tag := range tags {
		key := tag
		if i := strings.Index(tag, ":"); i >= 0 {
			key = tag[:i]
		}
		excluded := false
		for _, k := range f.keys {
			if k == key {
				excluded = true
				break
			}
		}
		if !excluded {
			kept = append(kept, tag)
		}
	}
	return kept
}

func filteredOrigin(t *testing.T, filter TagFilter) *Origin {
	t.Helper()
	cfg := &config.LogsConfig{
		Source:         "a",
		SourceCategory: "b",
		Tags:           []string{"env:prod", "dirname:/var/log"},
	}
	origin := NewOrigin(sources.NewLogSource("", cfg))
	origin.SetTags([]string{"kube_app_name:web", "service:web"})
	origin.SetTagFilters(filter)
	return origin
}

func TestTransportTagsAppliesExcludeFilter(t *testing.T) {
	origin := filteredOrigin(t, excludeKeys("dirname", "kube_app_name"))

	assert.Equal(t, []string{"service:web", "sourcecategory:b", "env:prod"}, origin.TransportTags())
	assert.Equal(t, "service:web,sourcecategory:b,env:prod", origin.TransportTagsToString())
}

func TestTagsAreNotFilteredByTagFilters(t *testing.T) {
	filter := excludeKeys("dirname", "kube_app_name")
	origin := filteredOrigin(t, filter)

	unfiltered := []string{"kube_app_name:web", "service:web", "sourcecategory:b", "env:prod", "dirname:/var/log"}
	assert.Equal(t, unfiltered, origin.Tags())
	assert.Equal(t, strings.Join(unfiltered, ","), origin.TagsToString())

	assert.Empty(t, filter.seen, "Tags()/TagsToString() must not invoke the tag filter")

	msg := NewMessage([]byte("hello"), origin, StatusInfo, 0)
	assert.Equal(t, unfiltered, msg.Tags())
	assert.Equal(t, strings.Join(unfiltered, ","), msg.TagsToString())
	assert.Empty(t, filter.seen, "MessageMetadata.Tags() must not invoke the tag filter")

	assert.Equal(t, []string{"service:web", "sourcecategory:b", "env:prod"}, msg.TransportTags())
	assert.Equal(t, "service:web,sourcecategory:b,env:prod", msg.TransportTagsToString())
}

func TestNilTagFilterIsNoOpOnBothPaths(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Origin)
	}{
		{"never set", func(*Origin) {}},
		{"set to nil", func(o *Origin) { o.SetTagFilters(nil) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.LogsConfig{
				Source:         "a",
				SourceCategory: "b",
				Tags:           []string{"env:prod", "dirname:/var/log"},
			}
			origin := NewOrigin(sources.NewLogSource("", cfg))
			origin.SetTags([]string{"kube_app_name:web", "service:web"})
			tc.setup(origin)

			assert.Equal(t, origin.Tags(), origin.TransportTags())
			assert.Equal(t, origin.TagsToString(), origin.TransportTagsToString())

			msg := NewMessage([]byte("hello"), origin, StatusInfo, 0)
			assert.Equal(t, msg.Tags(), msg.TransportTags())
			assert.Equal(t, msg.TagsToString(), msg.TransportTagsToString())

			assert.Equal(t,
				`[dd ddsource="a"][dd ddsourcecategory="b"][dd ddtags="env:prod,dirname:/var/log,kube_app_name:web,service:web"]`,
				string(origin.TagsPayload(nil)))
		})
	}
}

func TestTagsPayloadHonorsTagFilters(t *testing.T) {
	origin := filteredOrigin(t, excludeKeys("dirname", "kube_app_name", "processing"))

	assert.Equal(t,
		`[dd ddsource="a"][dd ddsourcecategory="b"][dd ddtags="env:prod,service:web,second:tag"]`,
		string(origin.TagsPayload([]string{"processing:tag", "second:tag"})))
}

func TestTagsPayloadDropsSourceCategoryWhenExcluded(t *testing.T) {
	origin := filteredOrigin(t, excludeKeys("sourcecategory"))

	assert.Equal(t, []string{"kube_app_name:web", "service:web", "env:prod", "dirname:/var/log"}, origin.TransportTags())
	assert.Equal(t,
		`[dd ddsource="a"][dd ddtags="env:prod,dirname:/var/log,kube_app_name:web,service:web"]`,
		string(origin.TagsPayload(nil)))
}

func TestOriginInheritsTheSourceTagFilter(t *testing.T) {
	origin := NewOrigin(sources.NewLogSource("", &config.LogsConfig{
		Tags:       []string{"env:prod", "dirname:/var/log"},
		TagFilters: &config.TagFilters{Exclude: []string{"dirname:*"}},
	}))
	origin.SetTags([]string{"kube_app_name:web"})

	assert.Equal(t, []string{"kube_app_name:web", "env:prod"}, origin.TransportTags())
	assert.Equal(t, []string{"kube_app_name:web", "env:prod", "dirname:/var/log"}, origin.Tags())
}

func TestTagsPayloadEmptyWhenFilterDropsEverything(t *testing.T) {
	origin := filteredOrigin(t, excludeKeys("dirname", "kube_app_name", "env", "service"))

	assert.Equal(t,
		`[dd ddsource="a"][dd ddsourcecategory="b"]`,
		string(origin.TagsPayload(nil)))
}

func TestTransportTagsDoesNotMutateOriginTags(t *testing.T) {
	cfg := &config.LogsConfig{Tags: []string{"env:prod"}}
	origin := NewOrigin(sources.NewLogSource("", cfg))

	tags := make([]string, 2, 10)
	tags[0] = "kube_app_name:web"
	tags[1] = "dirname:/var/log"
	origin.SetTags(tags)
	origin.SetTagFilters(excludeKeys("dirname"))

	assert.Equal(t, []string{"kube_app_name:web", "env:prod"}, origin.TransportTags())
	assert.Equal(t, []string{"kube_app_name:web", "dirname:/var/log"}, origin.tags)

	assert.Equal(t, []string{"kube_app_name:web", "env:prod"}, origin.TransportTags())
	assert.Equal(t, []string{"kube_app_name:web", "dirname:/var/log", "env:prod"}, origin.Tags())
}
