// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

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

// benchSource returns one shared source, already resolved against global, so the
// benchmark measures the steady-state per-message cost rather than compilation.
func benchSource(b *testing.B, global *tagfilter.Filters, sourceFilters *config.TagFilters) (*Processor, *sources.LogSource) {
	b.Helper()
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
		b.Fatal("source did not resolve; benchmark would measure the compile path")
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

func BenchmarkTagFilterGlobalNoMatch(b *testing.B) {
	global, _ := tagfilter.Compile(nil, []string{"nonexistent_key:*"})
	runParallel(b, global, nil)
}

func BenchmarkTagFilterGlobalRemoval(b *testing.B) {
	global, _ := tagfilter.Compile(nil, []string{
		"container_id:*", "kube_replica_set:*", "pod_name:*", "display_container_name:*",
	})
	runParallel(b, global, nil)
}

func BenchmarkTagFilterPerSource(b *testing.B) {
	global, _ := tagfilter.Compile(nil, []string{"container_id:*"})
	runParallel(b, global, &config.TagFilters{
		Include: []string{"container_id:*"},
		Exclude: []string{"filename:*", "dirname:*"},
	})
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
