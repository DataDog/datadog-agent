// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build test

package context_resolver

import (
	"encoding/json"
	// stdlib
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
	tb := util.NewTagsBuilder()
	sample.GetTags(tb)
	return k.Generate(sample.GetName(), sample.GetHost(), tb)
}

func TestGenerateContextKey(t *testing.T) {
	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	contextKey := generateContextKey(&mSample)
	assert.Equal(t, ckey.ContextKey(0xd28d2867c6dd822c), contextKey)
}

func TestTrackContext(t *testing.T) {
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{ // same as mSample2, with different Host
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
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
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
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
		Tags: []string{"bar", "bar"},
	})

	context, _ := resolver.Get(ckey)
	assert.Equal(t, len(context.Tags), 1)
	assert.Equal(t, context.Tags, []string{"bar"})
}

// TODO(remy): dedup this method which has been stolen in ckey pkg
func genTags(count int, div int) ([]string, []string) {
	var tags []string
	uniqMap := make(map[string]struct{})
	for i := 0; i < count; i++ {
		tag := fmt.Sprintf("tag%d:value%d", i/div, i/div)
		tags = append(tags, tag)
		uniqMap[tag] = struct{}{}
	}

	uniq := []string{}
	for tag := range uniqMap {
		uniq = append(uniq, tag)
	}

	return tags, uniq
}

func benchmarkContextResolverTrackContext(resolver ContextResolver, b *testing.B) {
	// track 2M contexts with 30 tags
	for contextsCount := 1<<15; contextsCount < 2<<21; contextsCount *= 2 {
		resolver.Clear()
		tags, _ := genTags(30, 1)

		fmt.Println("Overhead")
		ReportMemStats()

		b.Run(fmt.Sprintf("with-%d-contexts", contextsCount), func(b *testing.B) {
			fmt.Println("Start bench")
			b.ReportAllocs()
			j := 0
			for n := 0; n < b.N; n++ {
				key := resolver.TrackContext(&metrics.MetricSample{
					Name: fmt.Sprintf("metric.name%d", j),
					Tags: tags,
				})
				j++
				if j >= contextsCount {
					j = 0
				}
				// To make sure we don't make the get too expensive
				resolver.Get(key)
			}
			ReportMemStats()
		})
	}
}

func benchmarkContextResolverGetWorstCase(resolver ContextResolver, b *testing.B) {
	// track 2M contexts with 30 tags
	for contextsCount := 1<<15; contextsCount < 2<<21; contextsCount *= 2 {
		resolver.Clear()
		tags, _ := genTags(30, 1)

		ckeys := make([]ckey.ContextKey, 0)
		for i := 0; i < contextsCount; i++ {
			ckeys = append(ckeys, resolver.TrackContext(&metrics.MetricSample{
				Name: fmt.Sprintf("metric.name%d", i),
				Tags: tags,
			}))
		}

		// All the memory above should not be accounted for
		fmt.Println("Overhead")
		ReportMemStats()

		b.Run(fmt.Sprintf("with-%d-contexts", contextsCount), func(b *testing.B) {
			fmt.Println("Start bench")
			b.ReportAllocs()
			j := 0
			for n := 0; n < b.N; n++ {
				resolver.Get(ckeys[j])
				j++
				if j >= contextsCount {
					j = 0
				}
			}
			ReportMemStats()
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


func ReportMemStats() {
	var m MemReport
	var rtm runtime.MemStats

	// Read full mem stats after removing useless things.
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

	// Just encode to json and print
	b, _ := json.Marshal(m)
	fmt.Println(string(b))
}

func BenchmarkContextResolverTrackContextInMemory(b *testing.B) {
	benchmarkContextResolverTrackContext(NewContextResolverInMemory(), b)
}

func BenchmarkContextResolverTrackContextBadgerInMemory(b *testing.B) {
	resolver := NewContextResolverBadger(true, "")
	benchmarkContextResolverTrackContext(resolver, b)
}

func BenchmarkContextResolverTrackContextBadgerOnDisk(b *testing.B) {
	path, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	resolver := NewContextResolverBadger(false, path + "/db")
	benchmarkContextResolverTrackContext(resolver, b)
}

func BenchmarkContextResolverTrackContextBadgerInMemoryAndLRU(b *testing.B) {
	resolver := NewcontextResolverWithLRU(NewContextResolverBadger(true, ""), 1024)
	benchmarkContextResolverTrackContext(resolver, b)
}

func BenchmarkContextResolverGetWorstCaseInMemory(b *testing.B) {
	benchmarkContextResolverTrackContext(NewContextResolverInMemory(), b)
}

func BenchmarkContextResolverGetWorstCaseBadgerInMemory(b *testing.B) {
	resolver := NewContextResolverBadger(true, "")
	benchmarkContextResolverTrackContext(resolver, b)
}

func BenchmarkContextResolverGetWorstCaseBadgerOnDisk(b *testing.B) {
	path, err := ioutil.TempDir("", "badger")
	if err != nil {
		log.Fatal(err)
	}
	resolver := NewContextResolverBadger(false, path + "/db")
	benchmarkContextResolverTrackContext(resolver, b)
}

func BenchmarkContextResolverGetWorstCaseBadgerInMemoryAndLRU(b *testing.B) {
	resolver := NewcontextResolverWithLRU(NewContextResolverBadger(true, ""), 1024)
	benchmarkContextResolverTrackContext(resolver, b)
}
