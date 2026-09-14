// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// benchTags is a realistic containerized tag set.
func benchTags() []string {
	return []string{
		"container_id:9f8d7c6b5a4321009f8d7c6b5a4321009f8d7c6b5a4321009f8d7c6b5a4321",
		"container_name:myapp-container", "image_name:myapp", "image_tag:1.4.2",
		"short_image:myapp", "kube_namespace:default", "kube_deployment:myapp",
		"kube_replica_set:myapp-7c9f8d6b4", "pod_name:myapp-7c9f8d6b4-abcde",
		"kube_container_name:myapp", "kube_cluster_name:staging", "kube_service:myapp",
		"display_container_name:myapp-container_myapp-pod", "filename:app.log",
		"dirname:/var/log/pods", "env:prod", "service:myapp", "version:1.4.2",
		"team:infra", "log_hash:c0ffee",
	}
}

// benchSource returns one shared source, already resolved against global, so
// callers measure the steady-state per-message cost rather than compilation.
func benchSource(tb testing.TB, global *tagfilter.Filters, sourceFilters *config.TagFilters) (*Processor, *sources.LogSource) {
	tb.Helper()
	src := sources.NewLogSource("bench", &config.LogsConfig{
		Type:       config.FileType,
		Source:     "myapp",
		Service:    "myapp",
		Tags:       []string{"env:prod", "team:infra"},
		TagFilters: sourceFilters,
	})
	p := &Processor{tagFilters: global}
	ResolveSourceTagFilter(global, src)
	if _, ok := src.TagFilter(); !ok {
		tb.Fatal("fixture did not resolve; caller would measure the compile path")
	}
	return p, src
}

// runParallel drives the per-message tag-filter path from GOMAXPROCS goroutines
// against one shared source, matching how several pipeline processors contend on
// a single wildcard file source.
func runParallel(b *testing.B, global *tagfilter.Filters, sourceFilters *config.TagFilters) {
	p, src := benchSource(b, global, sourceFilters)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		origin := message.NewOrigin(src)
		origin.SetTags(benchTags())
		msg := message.NewMessage([]byte("bench"), origin, message.StatusInfo, 0)
		for pb.Next() {
			p.resolveTagFilter(msg)
		}
	})
}

// BenchmarkTagFilterDisabled is the regression gate: nothing configured anywhere.
func BenchmarkTagFilterDisabled(b *testing.B) {
	runParallel(b, nil, nil)
}

// BenchmarkTagFilterResolveCached is the cache-hit cost once a filter is
// configured: the same atomic load and generation check, however many rules back it.
func BenchmarkTagFilterResolveCached(b *testing.B) {
	global, _ := tagfilter.Compile(nil, []string{"container_id:*"})
	runParallel(b, global, &config.TagFilters{
		Include: []string{"container_id:*"},
		Exclude: []string{"filename:*", "dirname:*"},
	})
}

// benchNoMatchGlobal is configured but shares no key with benchTags.
func benchNoMatchGlobal() *tagfilter.Filters {
	global, _ := tagfilter.Compile(nil, []string{"nonexistent_key:*"})
	return global
}

// benchRemovalGlobal excludes four keys present in benchTags.
func benchRemovalGlobal() *tagfilter.Filters {
	global, _ := tagfilter.Compile(nil, []string{
		"container_id:*", "kube_replica_set:*", "pod_name:*", "display_container_name:*",
	})
	return global
}

// TestJSONEncodeFilterFixturesAreEngaged pins the Group 2 benchmark fixtures
// against a typo silently turning a filtering benchmark into a no-op one.
func TestJSONEncodeFilterFixturesAreEngaged(t *testing.T) {
	tags := benchTags()

	_, noMatchSrc := benchSource(t, benchNoMatchGlobal(), nil)
	noMatchFilter, _ := noMatchSrc.TagFilter()
	if noMatchFilter == nil {
		t.Fatal("no-match fixture resolved to a nil filter")
	}
	gotNoMatch := noMatchFilter.Keep(tags)
	assert.Equal(t, tags, gotNoMatch, "the no-match fixture must return tags unchanged")

	_, removalSrc := benchSource(t, benchRemovalGlobal(), nil)
	removalFilter, _ := removalSrc.TagFilter()
	if removalFilter == nil {
		t.Fatal("removal fixture resolved to a nil filter")
	}
	gotRemoval := removalFilter.Keep(tags)
	assert.Less(t, len(gotRemoval), len(gotNoMatch), "the removal fixture must drop tags the no-match fixture keeps")
}

// errSink defeats dead-code elimination for Encode's returned error.
var errSink error

// benchEncode times JSONEncoder.Encode for one global filter configuration: the
// filter is resolved once, then matched against the message's tags on every call.
func benchEncode(b *testing.B, global *tagfilter.Filters) {
	b.Helper()
	p, src := benchSource(b, global, nil)
	p.encoder = JSONEncoder
	filter, _ := src.TagFilter()

	origin := message.NewOrigin(src)
	origin.SetTags(benchTags())
	content := []byte("bench")
	msg := message.NewMessage(content, origin, message.StatusInfo, 0)
	msg.SetRendered(content)

	if err := p.encoder.Encode(msg, "bench-host", filter); err != nil {
		b.Fatalf("fixture failed to encode: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.SetRendered(content)
		errSink = p.encoder.Encode(msg, "bench-host", filter)
	}
}

func BenchmarkJSONEncodeFilterDisabled(b *testing.B) {
	benchEncode(b, nil)
}

func BenchmarkJSONEncodeFilterNoMatch(b *testing.B) {
	benchEncode(b, benchNoMatchGlobal())
}

func BenchmarkJSONEncodeFilterRemoval(b *testing.B) {
	benchEncode(b, benchRemovalGlobal())
}

// msgSink defeats dead-code elimination; without it the allocation is optimized
// away and the benchmark reports a sub-nanosecond, zero-allocation lie.
var msgSink *message.Message

// BenchmarkMessageAlloc measures the per-message allocation footprint, which is
// what a tagFilter field on MessageMetadata changes.
func BenchmarkMessageAlloc(b *testing.B) {
	src := sources.NewLogSource("bench", &config.LogsConfig{Type: config.FileType})
	origin := message.NewOrigin(src)
	content := []byte("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgSink = message.NewMessage(content, origin, message.StatusInfo, 0)
	}
}
