// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

func TestAddRequest(t *testing.T) {
	stats := NewRequestStats()
	stats.AddRequest(405, 10.0, 1, nil)
	stats.AddRequest(405, 15.0, 2, nil)
	stats.AddRequest(405, 20.0, 3, nil)

	assert.Nil(t, stats.Data[100])
	assert.Nil(t, stats.Data[200])
	assert.Nil(t, stats.Data[300])
	assert.Nil(t, stats.Data[500])
	s := stats.Data[405]

	if assert.NotNil(t, s) {
		assert.Equal(t, 3, s.Count)
		assert.Equal(t, 3.0, s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0)  // min item
		verifyQuantile(t, s.Latencies, 0.99, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0)  // max item
	}
}

func TestCombineWith(t *testing.T) {
	stats := NewRequestStats()
	for i := uint16(100); i <= 500; i += 100 {
		assert.Nil(t, stats.Data[i])
	}

	stats2 := NewRequestStats()
	stats3 := NewRequestStats()
	stats4 := NewRequestStats()
	stats2.AddRequest(405, 10.0, 1, nil)
	stats3.AddRequest(405, 15.0, 2, nil)
	stats4.AddRequest(405, 20.0, 4, nil)

	stats.CombineWith(stats2)
	stats.CombineWith(stats3)
	stats.CombineWith(stats4)

	assert.Nil(t, stats.Data[100])
	assert.Nil(t, stats.Data[200])
	assert.Nil(t, stats.Data[300])
	assert.Nil(t, stats.Data[500])
	s := stats.Data[405]

	if assert.NotNil(t, s) {
		assert.Equal(t, 3.0, s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
		assert.Equal(t, uint64(1|2|4), s.StaticTags)
	}
}

func TestCombineWithDiscovery(t *testing.T) {
	// Discovery buckets have no DDSketch and track LatencySum, even for Count>1.
	dst := NewRequestStats()
	src := NewRequestStats()
	for i := 0; i < 3; i++ {
		dst.AddDiscoveryRequest(200, 10.0, 1, nil)
	}
	for i := 0; i < 2; i++ {
		src.AddDiscoveryRequest(200, 20.0, 2, nil)
	}

	assert.NotPanics(t, func() { dst.CombineWith(src) })

	s := dst.Data[200]
	if assert.NotNil(t, s) {
		assert.Nil(t, s.Latencies, "discovery merge must not build a DDSketch")
		assert.Equal(t, 5, s.Count)
		assert.Equal(t, 10.0*3+20.0*2, s.LatencySum)
		assert.Equal(t, uint64(1|2), s.StaticTags)
	}
}

func TestCombineWithDiscoveryIntoEmpty(t *testing.T) {
	// Merging into a status the receiver has no bucket for creates it.
	dst := NewRequestStats()
	src := NewRequestStats()
	src.AddDiscoveryRequest(200, 10.0, 0, nil)
	src.AddDiscoveryRequest(200, 30.0, 0, nil)

	dst.CombineWith(src)

	s := dst.Data[200]
	if assert.NotNil(t, s) {
		assert.Nil(t, s.Latencies)
		assert.Equal(t, 2, s.Count)
		assert.Equal(t, 40.0, s.LatencySum)
	}
}

func TestCombineWithDiscoverySingleSample(t *testing.T) {
	// A single-sample discovery bucket carries LatencySum (FirstLatencySample==0);
	// the merge must keep LatencySum rather than routing through AddRequest.
	dst := NewRequestStats()
	src := NewRequestStats()
	src.AddDiscoveryRequest(200, 42.0, 0, nil)

	dst.CombineWith(src)

	s := dst.Data[200]
	if assert.NotNil(t, s) {
		assert.Equal(t, 1, s.Count)
		assert.Equal(t, 42.0, s.LatencySum)
		assert.Zero(t, s.FirstLatencySample)
		assert.Nil(t, s.Latencies)
	}
}

func TestCombineWithDiscoveryStatusClassesAndTags(t *testing.T) {
	// Both collapsed classes merge independently, with static/dynamic tags.
	dst := NewRequestStats()
	src := NewRequestStats()
	src.AddDiscoveryRequest(200, 10.0, 1, common.NewStringSet("svc:a"))
	src.AddDiscoveryRequest(500, 20.0, 2, common.NewStringSet("svc:b")) // collapses to 400

	dst.CombineWith(src)

	assert.Len(t, dst.Data, 2)

	if ok := dst.Data[200]; assert.NotNil(t, ok) {
		assert.Equal(t, 1, ok.Count)
		assert.Equal(t, 10.0, ok.LatencySum)
		assert.Equal(t, uint64(1), ok.StaticTags)
		_, has := ok.DynamicTags["svc:a"]
		assert.True(t, has)
	}
	if errBucket := dst.Data[400]; assert.NotNil(t, errBucket) {
		assert.Equal(t, 1, errBucket.Count)
		assert.Equal(t, 20.0, errBucket.LatencySum)
		assert.Equal(t, uint64(2), errBucket.StaticTags)
		_, has := errBucket.DynamicTags["svc:b"]
		assert.True(t, has)
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}

func benchmarkRequestStatsPool(b *testing.B, reqNum int) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats := NewRequestStats()
		for j := 0; j < reqNum; j++ {
			stats.AddRequest(405, 10.0, 1, nil)
		}
		stats.Close()
	}
}

func BenchmarkRequestStatsPool_10Reqs(b *testing.B) {
	benchmarkRequestStatsPool(b, 10)
}

func BenchmarkRequestStatsPool_100Reqs(b *testing.B) {
	benchmarkRequestStatsPool(b, 100)
}

func BenchmarkRequestStatsPool_1000Reqs(b *testing.B) {
	benchmarkRequestStatsPool(b, 1000)
}

func BenchmarkRequestStatsPool_10000Reqs(b *testing.B) {
	benchmarkRequestStatsPool(b, 10000)
}
