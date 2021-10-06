// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build test

package contextresolver

import (
	// stdlib
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"runtime"
	"testing"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
)

func generateContextKey(sample metrics.MetricSampleContext) ckey.ContextKey {
	k := ckey.NewKeyGenerator()
	tb := util.NewHashingTagsBuilder()
	sample.GetTags(tb)
	return k.Generate(sample.GetName(), sample.GetHost(), tb)
}

func TestGenerateContextKey(t *testing.T) {
	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "foo:bar", "bar:foo"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	contextKey := generateContextKey(&mSample)
	assert.Equal(t, ckey.ContextKey(0x70759ff7dfd6914c), contextKey)
}

func TestTrackContext(t *testing.T) {
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "foo:bar", "bar:foo"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz", "foo:bar", "bar:foo"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{ // same as mSample2, with different Host
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz", "foo:bar", "bar:foo"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}
	expectedContext1 := Context{
		Name: mSample1.Name,
		Tags: mSample1.Tags,
	}
	expectedContext2 := Context{
		Name: mSample2.Name,
		Tags: mSample2.Tags,
	}
	expectedContext3 := Context{
		Name: mSample3.Name,
		Tags: mSample3.Tags,
		Host: mSample3.Host,
	}
	contextResolver := newContextResolver()

	// Track the 2 contexts
	contextKey1 := contextResolver.TrackContext(&mSample1)
	contextKey2 := contextResolver.TrackContext(&mSample2)
	contextKey3 := contextResolver.TrackContext(&mSample3)

	// When we look up the 2 keys, they return the correct contexts
	context1, ok := contextResolver.Get(contextKey1)
	assert.Equal(t, expectedContext1, *context1)

	context2, ok := contextResolver.Get(contextKey2)
	assert.Equal(t, expectedContext2, *context2)

	context3, ok := contextResolver.Get(contextKey3)
	assert.Equal(t, expectedContext3, *context3)

	unknownContextKey := ckey.ContextKey(0xffffffffffffffff)
	_, ok = contextResolver.Get(unknownContextKey)
	assert.False(t, ok)
}

func TestExpireContexts(t *testing.T) {
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "foo:bar", "bar:foo"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz", "foo:bar", "bar:foo"},
		SampleRate: 1,
	}
	contextResolver := NewTimestampContextResolver()

	// Track the 2 contexts
	contextKey1 := contextResolver.TrackContext(&mSample1, 4)
	contextKey2 := contextResolver.TrackContext(&mSample2, 6)

	// With an expireTimestap of 3, both contexts are still valid
	assert.Len(t, contextResolver.ExpireContexts(3), 0)
	_, ok1 := contextResolver.resolver.Get(contextKey1)
	_, ok2 := contextResolver.resolver.Get(contextKey2)
	assert.True(t, ok1)
	assert.True(t, ok2)

	// With an expireTimestap of 5, context 1 is expired
	expiredContextKeys := contextResolver.ExpireContexts(5)
	if assert.Len(t, expiredContextKeys, 1) {
		assert.Equal(t, contextKey1, expiredContextKeys[0])
	}

	// context 1 is not tracked anymore, but context 2 still is
	_, ok := contextResolver.resolver.Get(contextKey1)
	assert.False(t, ok)
	_, ok = contextResolver.resolver.Get(contextKey2)
	assert.True(t, ok)
}

func TestCountBasedExpireContexts(t *testing.T) {
	mSample1 := metrics.MetricSample{Name: "my.metric.name1"}
	mSample2 := metrics.MetricSample{Name: "my.metric.name2"}
	mSample3 := metrics.MetricSample{Name: "my.metric.name3"}
	contextResolver := NewCountBasedContextResolver(2)
	defer contextResolver.Close()

	contextKey1 := contextResolver.TrackContext(&mSample1)
	contextKey2 := contextResolver.TrackContext(&mSample2)
	require.Len(t, contextResolver.ExpireContexts(), 0)

	contextKey3 := contextResolver.TrackContext(&mSample3)
	contextResolver.TrackContext(&mSample2)
	require.Len(t, contextResolver.ExpireContexts(), 0)

	expiredContextKeys := contextResolver.ExpireContexts()
	require.ElementsMatch(t, expiredContextKeys, []ckey.ContextKey{contextKey1})

	expiredContextKeys = contextResolver.ExpireContexts()
	require.ElementsMatch(t, expiredContextKeys, []ckey.ContextKey{contextKey2, contextKey3})

	require.Len(t, contextResolver.ExpireContexts(), 0)
	require.Equal(t, 0, contextResolver.resolver.Size())
}

func TestTagDeduplication(t *testing.T) {
	resolver := newContextResolver()

	ckey := resolver.TrackContext(&metrics.MetricSample{
		Name: "foo",
		Tags: []string{"bar", "bar", "bar:1"},
	})

	context, _ := resolver.Get(ckey)
	assert.Equal(t, len(context.Tags), 2)
	assert.Equal(t, context.Tags, []string{"bar", "bar:1"})
}

// genDupTags generates tags with potential duplicates if div > 1
func genDupTags(count int, div int) []string {
	tags := make([]string, count)

	for i := 0; i < count; i++ {
		tag := fmt.Sprintf("tag%d:value%d", i/div, i/div)
		tags[i] = tag
	}

	return tags
}

// genTags generates tags and the value can be made unique by setting a seed
func genTags(count int64, seed int64) []string {
	tags := make([]string, count)

	for i := int64(0); i < count; i++ {
		tag := fmt.Sprintf("tagname%d:tagvalue%d", i, i * seed * 12345 + seed)
		tags[i] = tag
	}

	return tags
}

// Run this with -test.benchtime=5s otherwise it won't generate all the contexts!
func benchmarkContextResolverTrackContextManyMetrics(resolver ContextResolver, b *testing.B) {
	// track 2M contexts with the 30 same tags
	for contextsCount := int64(1 << 15); contextsCount < int64(2<<21); contextsCount *= 2 {
		resolver.Clear()
		tags := genDupTags(30, 1)

		b.Run(fmt.Sprintf("with-%d-contexts", contextsCount), func(b *testing.B) {
			b.ReportAllocs()
			j := int64(0)
			for n := 0; n < b.N; n++ {
				var key ckey.ContextKey
				{
					sample := &metrics.MetricSample{
						Name: fmt.Sprintf("metric.name%d", j),
						Tags: tags,
					}
					key = resolver.TrackContext(sample)
				}
				j++
				if j >= contextsCount {
					j = 0
				}
				// To make sure we don't make the get too expensive
				resolver.Get(key)
			}
			ReportMemStats(b)
		})
	}
}

// Run this with -test.benchtime=5s otherwise it won't generate all the contexts!
func benchmarkContextResolverTrackContextManyTags(resolver ContextResolver, b *testing.B) {
	// track 2M contexts with one metrics and 30 tags with unique values
	for contextsCount := int64(1 << 15); contextsCount < int64(2<<21); contextsCount *= 2 {
		resolver.Clear()

		j := int64(0)
		b.Run(fmt.Sprintf("with-%d-contexts", contextsCount), func(b *testing.B) {

			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				var key ckey.ContextKey
				{
					sample := &metrics.MetricSample{
						Name: "metric.name",
						Tags: genTags(30, j),
					}
					key = resolver.TrackContext(sample)
				}
				j++
				if j >= contextsCount {
					j = 0
				}
				// To make sure we don't make the get too expensive
				resolver.Get(key)
			}
			ReportMemStats(b)
		})
		fmt.Printf("test")
		break
	}
}

func benchmarkContextResolverGetWorstCase(resolver ContextResolver, b *testing.B) {
	// track 2M contexts with 30 tags
	for contextsCount := 1 << 15; contextsCount < 2<<21; contextsCount *= 2 {
		resolver.Clear()
		tags := genDupTags(30, 1)

		ckeys := make([]ckey.ContextKey, 0)
		for i := 0; i < contextsCount; i++ {
			ckeys = append(ckeys, resolver.TrackContext(&metrics.MetricSample{
				Name: fmt.Sprintf("metric.name%d", i),
				Tags: tags,
			}))
		}

		b.Run(fmt.Sprintf("with-%d-contexts", contextsCount), func(b *testing.B) {
			b.ReportAllocs()
			j := 0
			for n := 0; n < b.N; n++ {
				resolver.Get(ckeys[j])
				j++
				if j >= contextsCount {
					j = 0
				}
			}
			ReportMemStats(b)
		})
	}
}

type MemReport struct {
	Alloc,
	TotalAlloc,
	Sys,
	Mallocs,
	Frees,
	HeapInUse,
	LiveObjects,
	PauseTotalNs uint64

	NumGC        uint32
	NumGoroutine int
}

func ReportMemStats(b *testing.B) {
	var m MemReport
	var rtm runtime.MemStats

	// Read full mem stats after removing useless things.
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&rtm)

	// Number of goroutines
	m.NumGoroutine = runtime.NumGoroutine()

	// Misc memory stats
	m.Alloc = rtm.Alloc
	m.TotalAlloc = rtm.TotalAlloc
	m.Sys = rtm.Sys
	m.Mallocs = rtm.Mallocs
	m.Frees = rtm.Frees
	m.HeapInUse = rtm.HeapInuse

	// Live objects = Mallocs - Frees
	m.LiveObjects = (m.Mallocs - m.Frees)

	// GC Stats
	m.PauseTotalNs = rtm.PauseTotalNs
	m.NumGC = rtm.NumGC

	if (b != nil) {
		b.ReportMetric(float64(m.HeapInUse / 1024 / 1024), "heap_mb")
		b.ReportMetric(float64(m.LiveObjects / 1000), "k_objects")
	} else {
		s, _ := json.Marshal(m)
		fmt.Println(string(s))
	}
}

func BenchmarkContextResolverTrackContextManyMetricsInMemory(b *testing.B) {
	benchmarkContextResolverTrackContextManyMetrics(NewInMemory(), b)
}

func BenchmarkContextResolverTrackContextManyMetricsDedup(b *testing.B) {
	benchmarkContextResolverTrackContextManyMetrics(NewDedup(), b)
}

func BenchmarkContextResolverTrackContextManyMetricsBadgerInMemory(b *testing.B) {
	resolver := NewBadger(true, "")
	benchmarkContextResolverTrackContextManyMetrics(resolver, b)
}

func BenchmarkContextResolverTrackContextManyMetricsBadgerOnDisk(b *testing.B) {
	path, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	resolver := NewBadger(false, path+"/db")
	benchmarkContextResolverTrackContextManyMetrics(resolver, b)
}

func BenchmarkContextResolverTrackContextManyMetricsBadgerInMemoryAndLRU(b *testing.B) {
	resolver := NewWithLRU(NewBadger(true, ""), 1024)
	benchmarkContextResolverTrackContextManyMetrics(resolver, b)
}

func BenchmarkContextResolverTrackContextManyTagsInMemory(b *testing.B) {
	benchmarkContextResolverTrackContextManyTags(NewInMemory(), b)
}

func BenchmarkContextResolverTrackContextManyTagsDedup(b *testing.B) {
	benchmarkContextResolverTrackContextManyTags(NewDedup(), b)
}

func BenchmarkContextResolverGetWorstCaseInMemory(b *testing.B) {
	benchmarkContextResolverGetWorstCase(NewInMemory(), b)
}

func BenchmarkContextResolverGetWorstCaseDedup(b *testing.B) {
	benchmarkContextResolverGetWorstCase(NewDedup(), b)
}

func BenchmarkContextResolverGetWorstCaseBadgerInMemory(b *testing.B) {
	resolver := NewBadger(true, "")
	benchmarkContextResolverGetWorstCase(resolver, b)
}

func BenchmarkContextResolverGetWorstCaseBadgerOnDisk(b *testing.B) {
	path, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	resolver := NewBadger(false, path+"/db")
	benchmarkContextResolverGetWorstCase(resolver, b)
}

func BenchmarkContextResolverGetWorstCaseBadgerInMemoryAndLRU(b *testing.B) {
	resolver := NewWithLRU(NewBadger(true, ""), 1024)
	benchmarkContextResolverGetWorstCase(resolver, b)
}
