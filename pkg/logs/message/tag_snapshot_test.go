// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// countFallbackMerges installs fallbackMergeHook for the duration of the test
// and returns a function reporting how many fallback merges ran. Tests that
// use it must not run in parallel.
func countFallbackMerges(t *testing.T) func() int {
	t.Helper()
	var mu sync.Mutex
	count := 0
	fallbackMergeHook = func() {
		mu.Lock()
		count++
		mu.Unlock()
	}
	t.Cleanup(func() { fallbackMergeHook = nil })
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

func readAllTagViews(o *Origin) {
	_ = o.Tags()
	_ = o.TagsToString()
	_ = o.TagsPayload(nil)
	_ = o.TagsPayload(parityProcessingTags)
}

func newSnapshotTestOrigin(cfg *config.LogsConfig, attached []string) *Origin {
	origin := NewOrigin(sources.NewLogSource("", cfg))
	origin.SetTags(attached)
	return origin
}

// TestTagWireParityWithSnapshot runs the pinned P2-1 cases through the
// snapshot path: same literals, zero fallback merges.
func TestTagWireParityWithSnapshot(t *testing.T) {
	for _, c := range tagParityCases {
		t.Run(c.name, func(t *testing.T) {
			fallbacks := countFallbackMerges(t)
			origin := newParityOrigin(c)
			origin.BuildTagSnapshot()
			if c.mappedSource != "" {
				// remap_source runs in the processor, after the snapshot.
				origin.SetMappedSource(c.mappedSource)
			}
			assertTagParity(t, c, origin)
			assert.Zero(t, fallbacks(), "snapshot-carrying origin must not merge the live config")
		})
	}
}

// TestTagSnapshotMatchesFallback compares the snapshot path to today's merge
// over the full fixture matrix.
func TestTagSnapshotMatchesFallback(t *testing.T) {
	sourcesAxis := []string{"", "nginx"}
	mappedAxis := []string{"", "nginx-remapped"}
	categoryAxis := []string{"", "web"}
	configAxis := [][]string{nil, {}, {"env:prod", "team:infra"}, {"container_name:nginx"}}
	attachedAxis := [][]string{
		nil,
		{},
		{"filename:app.log", "dirname:/var/log", "container_name:nginx", "truncated:single_line"},
		{"truncated:single_line", "noisy_log:true", "container_name:nginx", "image_name:nginx"},
		{"truncated:single_line"},
		{"container_name:nginx", "host:journald-host"},
		{"truncated:true"},
	}
	processingAxis := [][]string{nil, {}, {"processing:tag", "second:tag"}}

	for _, src := range sourcesAxis {
		for _, mapped := range mappedAxis {
			for _, category := range categoryAxis {
				for ci, configTags := range configAxis {
					for ai, attached := range attachedAxis {
						name := fmt.Sprintf("source=%q/mapped=%q/category=%q/config=%d/attached=%d", src, mapped, category, ci, ai)
						t.Run(name, func(t *testing.T) {
							newOrigin := func() *Origin {
								return newSnapshotTestOrigin(&config.LogsConfig{
									Source:         src,
									SourceCategory: category,
									Tags:           configTags,
								}, attached)
							}
							fallback := newOrigin()
							snapshot := newOrigin()
							snapshot.BuildTagSnapshot()
							if mapped != "" {
								fallback.SetMappedSource(mapped)
								snapshot.SetMappedSource(mapped)
							}
							assertSameTagViews(t, fallback, snapshot, processingAxis)
						})
					}
				}
			}
		}
	}
}

func assertSameTagViews(t *testing.T, fallback, snapshot *Origin, processingAxis [][]string) {
	t.Helper()
	require.NotNil(t, snapshot.snapshot)
	assert.Equal(t, fallback.Tags(), snapshot.Tags(), "Tags()")
	assert.Equal(t, fallback.TagsToString(), snapshot.TagsToString(), "TagsToString()")
	for _, processing := range processingAxis {
		want := fallback.TagsPayload(processing)
		got := snapshot.TagsPayload(processing)
		assert.Equal(t, string(want), string(got), "TagsPayload(%v)", processing)
		assert.NotNil(t, got, "TagsPayload is never nil")
	}
}

// FuzzTagSnapshotMatchesFallback checks the snapshot against today's merge on
// arbitrary tag groups. Each string argument is a comma-separated tag list;
// an empty string means no tags.
func FuzzTagSnapshotMatchesFallback(f *testing.F) {
	f.Add("nginx", "", "web", "env:prod,team:infra", "filename:app.log,truncated:single_line", "processing:tag")
	f.Add("", "", "", "", "", "")
	f.Add("nginx", "remapped", "", "container_name:nginx", "container_name:nginx", "")
	f.Fuzz(func(t *testing.T, src, mapped, category, configCSV, attachedCSV, processingCSV string) {
		split := func(csv string) []string {
			if csv == "" {
				return nil
			}
			return strings.Split(csv, ",")
		}
		newOrigin := func() *Origin {
			return newSnapshotTestOrigin(&config.LogsConfig{
				Source:         src,
				SourceCategory: category,
				Tags:           split(configCSV),
			}, split(attachedCSV))
		}
		fallback := newOrigin()
		snapshot := newOrigin()
		snapshot.BuildTagSnapshot()
		if mapped != "" {
			fallback.SetMappedSource(mapped)
			snapshot.SetMappedSource(mapped)
		}
		assertSameTagViews(t, fallback, snapshot, [][]string{nil, split(processingCSV)})
	})
}

// TestTagSnapshotServesAllReadMethods proves that Tags, TagsToString and
// TagsPayload all read the snapshot: the fallback merge never runs.
func TestTagSnapshotServesAllReadMethods(t *testing.T) {
	fallbacks := countFallbackMerges(t)

	origin := newSnapshotTestOrigin(&config.LogsConfig{
		Source:         "nginx",
		SourceCategory: "web",
		Tags:           []string{"env:prod"},
	}, []string{"truncated:single_line"})

	// Sanity: without a snapshot each read merges the live config.
	readAllTagViews(origin)
	require.Equal(t, 4, fallbacks(), "the hook must count every fallback merge")

	origin.BuildTagSnapshot()
	readAllTagViews(origin)
	assert.Equal(t, 4, fallbacks(), "no read may fall back once a snapshot is built")
}

// TestTagSnapshotEmptyIsStillASnapshot proves presence is not inferred from
// emptiness: an empty snapshot is served, with today's empty values.
func TestTagSnapshotEmptyIsStillASnapshot(t *testing.T) {
	fallbacks := countFallbackMerges(t)

	origin := newSnapshotTestOrigin(&config.LogsConfig{}, nil)
	origin.BuildTagSnapshot()
	require.NotNil(t, origin.snapshot)

	assert.Equal(t, []string{}, origin.Tags())
	assert.NotNil(t, origin.Tags())
	assert.Equal(t, "", origin.TagsToString())
	assert.Equal(t, []byte{}, origin.TagsPayload(nil))
	assert.NotNil(t, origin.TagsPayload(nil))
	assert.Equal(t, `[dd ddtags="processing:tag,second:tag"]`, string(origin.TagsPayload(parityProcessingTags)))
	assert.Zero(t, fallbacks())
}

// TestTagSnapshotDefensiveCopies mutates every input slice after the build
// and checks the snapshot's output does not move.
func TestTagSnapshotDefensiveCopies(t *testing.T) {
	cfg := &config.LogsConfig{
		Source:         "nginx",
		SourceCategory: "web",
		Tags:           []string{"env:prod", "team:infra"},
	}
	parsing := ParsingExtra{Tags: make([]string, 1, 8)}
	parsing.Tags[0] = "truncated:single_line"
	attached := make([]string, 0, 8) // spare capacity exposes aliasing
	attached = append(attached, "container_name:nginx")
	attached = append(attached, parsing.Tags...)

	origin := newSnapshotTestOrigin(cfg, attached)
	origin.BuildTagSnapshot()

	wantTags := []string{"container_name:nginx", "truncated:single_line", "sourcecategory:web", "env:prod", "team:infra"}
	wantPayload := `[dd ddsource="nginx"][dd ddsourcecategory="web"][dd ddtags="env:prod,team:infra,container_name:nginx,truncated:single_line"]`
	require.Equal(t, wantTags, origin.Tags())

	// o.tags: overwrite in place and append into its spare capacity.
	origin.tags[0] = "mutated:attached"
	_ = append(origin.tags, "appended:attached")
	// Config.Tags: overwrite in place, then replace, then change the category.
	cfg.Tags[0] = "mutated:config"
	cfg.Tags = append(cfg.Tags, "appended:config")
	cfg.SourceCategory = "mutated"
	// ParsingExtra.Tags: overwrite in place and append.
	parsing.Tags[0] = "mutated:parsing"
	parsing.Tags = append(parsing.Tags, "appended:parsing")

	assert.Equal(t, wantTags, origin.Tags())
	assert.Equal(t, strings.Join(wantTags, ","), origin.TagsToString())
	assert.Equal(t, wantPayload, string(origin.TagsPayload(nil)))
}

// TestTagSnapshotSharesBackingArray documents the cached-slice contract: two
// Tags calls return the same backing array (no per-read allocation).
func TestTagSnapshotSharesBackingArray(t *testing.T) {
	origin := newSnapshotTestOrigin(&config.LogsConfig{Tags: []string{"env:prod"}}, []string{"a:b"})
	origin.BuildTagSnapshot()

	a := origin.Tags()
	b := origin.Tags()
	require.NotEmpty(t, a)
	assert.Same(t, &a[0], &b[0], "snapshot Tags() must return the shared slice")
	// cap == len: a caller that appends to Tags() always gets a new array and
	// can never write into the shared one.
	assert.Equal(t, len(a), cap(a))

	// The fallback path still hands out a private copy per call.
	fallback := newSnapshotTestOrigin(&config.LogsConfig{Tags: []string{"env:prod"}}, []string{"a:b"})
	c := fallback.Tags()
	d := fallback.Tags()
	assert.NotSame(t, &c[0], &d[0])
}

// TestTagSnapshotGroupsAreCapped checks that appending to a group slice can
// never write into the merged slice readers share.
func TestTagSnapshotGroupsAreCapped(t *testing.T) {
	s := newTagSnapshot([]string{"a:1", "b:2"}, []string{"c:3"}, "web")
	require.Equal(t, []string{"a:1", "b:2", "sourcecategory:web", "c:3"}, s.merged)
	assert.Equal(t, []string{"a:1", "b:2"}, s.attached)
	assert.Equal(t, []string{"c:3"}, s.configured)
	assert.Equal(t, len(s.attached), cap(s.attached))
	assert.Equal(t, len(s.configured), cap(s.configured))

	_ = append(s.attached, "x:x")
	_ = append(s.configured, "y:y")
	assert.Equal(t, []string{"a:1", "b:2", "sourcecategory:web", "c:3"}, s.merged)
}

// TestTagSnapshotSetTagsInvalidates proves a late tag writer is not silently
// ignored: SetTags drops the snapshot and the origin falls back.
func TestTagSnapshotSetTagsInvalidates(t *testing.T) {
	fallbacks := countFallbackMerges(t)

	origin := newSnapshotTestOrigin(&config.LogsConfig{SourceCategory: "web", Tags: []string{"env:prod"}}, []string{"a:b"})
	origin.BuildTagSnapshot()
	origin.SetTags([]string{"a:b", "noisy_log:true"})

	assert.Nil(t, origin.snapshot)
	assert.Equal(t, []string{"a:b", "noisy_log:true", "sourcecategory:web", "env:prod"}, origin.Tags())
	assert.Equal(t, `[dd ddsourcecategory="web"][dd ddtags="env:prod,a:b,noisy_log:true"]`, string(origin.TagsPayload(nil)))
	assert.Equal(t, 2, fallbacks())

	// Building again serves the new tags from a snapshot.
	origin.BuildTagSnapshot()
	assert.Equal(t, []string{"a:b", "noisy_log:true", "sourcecategory:web", "env:prod"}, origin.Tags())
	assert.Equal(t, 2, fallbacks())
}

// TestTagSnapshotSourceAndServiceStayLive proves remap_source and service
// changes after the build still reach the wire.
func TestTagSnapshotSourceAndServiceStayLive(t *testing.T) {
	cfg := &config.LogsConfig{SourceCategory: "web"}
	origin := newSnapshotTestOrigin(cfg, []string{"a:b"})
	origin.SetSource("parsed-app")
	origin.SetService("parsed-app")
	origin.BuildTagSnapshot()

	assert.Equal(t, `[dd ddsource="parsed-app"][dd ddsourcecategory="web"][dd ddtags="a:b"]`, string(origin.TagsPayload(nil)))

	origin.SetMappedSource("nginx-remapped")
	assert.Equal(t, "nginx-remapped", origin.Source())
	assert.Equal(t, `[dd ddsource="nginx-remapped"][dd ddsourcecategory="web"][dd ddtags="a:b"]`, string(origin.TagsPayload(nil)))
	assert.Equal(t, "parsed-app", origin.Service())
	// Tags are unaffected by the source.
	assert.Equal(t, []string{"a:b", "sourcecategory:web"}, origin.Tags())
}

// TestTagSnapshotConfiguredTagsAppearOnce is the snapshot variant of PR0's
// "configured tags appear once" regression: tailers must not pass
// Config.Tags into SetTags, and the snapshot must not add them twice.
func TestTagSnapshotConfiguredTagsAppearOnce(t *testing.T) {
	origin := newSnapshotTestOrigin(&config.LogsConfig{
		SourceCategory: "b",
		Tags:           []string{"team:infra"},
	}, []string{})
	origin.BuildTagSnapshot()

	assert.Equal(t, []string{"sourcecategory:b", "team:infra"}, origin.Tags())
	assert.Equal(t, `[dd ddsourcecategory="b"][dd ddtags="team:infra"]`, string(origin.TagsPayload(nil)))
}

// TestTagSnapshotNoSource checks BuildTagSnapshot is a no-op where the origin
// cannot hold a snapshot.
func TestTagSnapshotNoSource(t *testing.T) {
	var nilOrigin *Origin
	assert.NotPanics(t, nilOrigin.BuildTagSnapshot)

	noSource := &Origin{}
	noSource.BuildTagSnapshot()
	assert.Nil(t, noSource.snapshot)
	assert.Nil(t, noSource.Tags())
	assert.Equal(t, []byte{}, noSource.TagsPayload(nil))

	noConfig := NewOrigin(sources.NewLogSource("no-config", nil))
	assert.NotPanics(t, noConfig.BuildTagSnapshot)
	assert.Nil(t, noConfig.snapshot)
}

// TestTagSnapshotConcurrentConfigMutation proves the in-place Config.Tags
// race is gone for snapshot-carrying messages: readers never touch the live
// config, so -race stays quiet while a writer mutates it.
func TestTagSnapshotConcurrentConfigMutation(t *testing.T) {
	cfg := &config.LogsConfig{Source: "nginx", SourceCategory: "web", Tags: []string{"env:prod", "team:infra"}}
	origin := newSnapshotTestOrigin(cfg, []string{"a:b"})
	origin.BuildTagSnapshot()
	wantTags := []string{"a:b", "sourcecategory:web", "env:prod", "team:infra"}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cfg.Tags[0] = fmt.Sprintf("env:%d", i)
		}
	}()
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				assert.Equal(t, wantTags, origin.Tags())
				_ = origin.TagsPayload(nil)
			}
		}()
	}
	wg.Wait()
}
