// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		if f.Retains(tag) {
			kept = append(kept, tag)
		}
	}
	return kept
}

func (f *excludeKeysFilter) Retains(tag string) bool {
	key := tag
	if i := strings.Index(tag, ":"); i >= 0 {
		key = tag[:i]
	}
	for _, k := range f.keys {
		if k == key {
			return false
		}
	}
	return true
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

func newBenchOrigin(exclude []string) *Origin {
	cfg := &config.LogsConfig{
		Source:         "nginx",
		SourceCategory: "http",
		Tags:           []string{"env:prod", "version:1.2.3"},
	}
	if len(exclude) > 0 {
		cfg.TagFilters = &config.TagFilters{Exclude: exclude}
	}
	origin := NewOrigin(sources.NewLogSource("bench", cfg))
	origin.SetTags([]string{
		"filename:access.log",
		"dirname:/var/log/nginx",
		"kube_namespace:default",
		"kube_deployment:web",
		"pod_name:web-7d8f9c5b4-abcde",
		"container_id:a1b2c3d4e5f6",
		"image_name:nginx",
	})
	return origin
}

func benchOrigin(b *testing.B, exclude []string) *Origin {
	b.Helper()
	return newBenchOrigin(exclude)
}

func benchOriginForTest(t *testing.T, exclude []string) *Origin {
	t.Helper()
	return newBenchOrigin(exclude)
}

var benchExclude = []string{"dirname:*", "kube_*", "container_id"}

func BenchmarkTransportTagsToString(b *testing.B) {
	origin := benchOrigin(b, benchExclude)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = origin.TransportTagsToString()
	}
}

func BenchmarkTransportTagsToStringUnfiltered(b *testing.B) {
	origin := benchOrigin(b, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = origin.TransportTagsToString()
	}
}

func BenchmarkTransportTags(b *testing.B) {
	origin := benchOrigin(b, benchExclude)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = origin.TransportTags()
	}
}

func BenchmarkTagsPayload(b *testing.B) {
	origin := benchOrigin(b, benchExclude)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = origin.TagsPayload(nil)
	}
}

func TestBenchOriginFilterIsEngaged(t *testing.T) {
	filtered := benchOriginForTest(t, benchExclude)
	require.NotNil(t, filtered.TagFilters(), "benchmark origin must have a live filter")

	got := filtered.TransportTags()
	assert.NotContains(t, got, "dirname:/var/log/nginx")
	assert.NotContains(t, got, "kube_namespace:default")
	assert.NotContains(t, got, "container_id:a1b2c3d4e5f6")
	assert.Contains(t, got, "filename:access.log")
	assert.Contains(t, got, "sourcecategory:http")
	assert.Less(t, len(got), len(filtered.Tags()), "the filter must actually drop something")
}

func TestTransportTagsGroupOrderIsPreserved(t *testing.T) {
	origin := newBenchOrigin([]string{"dirname:*", "kube_*"})

	// `dirname:*` and `kube_*` drop three tags; pod_name, container_id and
	// image_name are not matched by either pattern and must survive.
	want := []string{
		"filename:access.log",
		"pod_name:web-7d8f9c5b4-abcde",
		"container_id:a1b2c3d4e5f6",
		"image_name:nginx",
		"sourcecategory:http",
		"env:prod",
		"version:1.2.3",
	}
	assert.Equal(t, want, origin.TransportTags(),
		"order is origin tags, then sourcecategory, then config tags")
	assert.Equal(t, strings.Join(want, ","), origin.TransportTagsToString())
}

func TestAppendTransportTagsAppendsToCallerSlice(t *testing.T) {
	origin := newBenchOrigin([]string{"dirname:*", "kube_*"})

	got := origin.appendTransportTags([]string{"pre:existing"})

	assert.Equal(t, []string{
		"pre:existing",
		"filename:access.log",
		"pod_name:web-7d8f9c5b4-abcde",
		"container_id:a1b2c3d4e5f6",
		"image_name:nginx",
		"sourcecategory:http",
		"env:prod",
		"version:1.2.3",
	}, got)
}
