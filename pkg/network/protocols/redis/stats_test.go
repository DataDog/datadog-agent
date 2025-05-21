// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/sketches-go/ddsketch"
)

func TestAddRequest(t *testing.T) {
	stats := NewRequestStats()
	stats.AddRequest(RedisNoErr, 10, 1, 10.0)
	stats.AddRequest(RedisNoErr, 15, 2, 15.0)
	stats.AddRequest(RedisNoErr, 20, 3, 20.0)

	s := stats.ErrorToStats[RedisNoErr]

	if assert.NotNil(t, s) {
		assert.Equal(t, 45, s.Count)
		assert.Equal(t, float64(45), s.Latencies.GetCount())
		assert.Equal(t, 10.0, s.FirstLatencySample)

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
	}
}

func TestCombineWith(t *testing.T) {

	stats := NewRequestStats()
	stats2 := NewRequestStats()
	stats3 := NewRequestStats()
	stats4 := NewRequestStats()

	stats2.AddRequest(RedisNoErr, 10, 1, 10.0)
	stats3.AddRequest(RedisNoErr, 15, 2, 15.0)
	stats4.AddRequest(RedisNoErr, 20, 3, 20.0)

	stats.CombineWith(stats2)
	stats.CombineWith(stats3)
	stats.CombineWith(stats4)

	s := stats.ErrorToStats[RedisNoErr]

	if assert.NotNil(t, s) {
		assert.Equal(t, 45, s.Count)
		assert.Equal(t, float64(45), s.Latencies.GetCount())
		assert.Equal(t, 10.0, s.FirstLatencySample)

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.GreaterOrEqual(t, val, expectedValue-acceptableError)
	assert.LessOrEqual(t, val, expectedValue+acceptableError)
}

func benchmarkRequestStatsPool(b *testing.B, reqNum int) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats := NewRequestStats()
		for j := 0; j < reqNum; j++ {
			stats.AddRequest(RedisNoErr, 1, 1, 1)
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
