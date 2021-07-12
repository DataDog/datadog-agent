// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build zlib

package serializer

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
)

func generateData(points int, items int, tags int) metrics.Series {
	series := metrics.Series{}
	for i := 0; i < items; i++ {
		series = append(series, &metrics.Serie{
			Points: func() []metrics.Point {
				ps := make([]metrics.Point, points)
				for p := 0; p < points; p++ {
					ps[p] = metrics.Point{Ts: float64(p * i), Value: float64(p + i)}
				}
				return ps
			}(),
			MType:    metrics.APIGaugeType,
			Name:     "test.metrics",
			Interval: 15,
			Host:     "localHost",
			Tags: func() []string {
				ts := make([]string, tags)
				for t := 0; t < tags; t++ {
					ts[t] = fmt.Sprintf("tag%d:foobar", t)
				}
				return ts
			}(),
		})
	}
	return series
}

func benchmarkJSONPayloadBuilderUsage(b *testing.B, points int, items int, tags int) {

	series := generateData(points, items, tags)
	payloadBuilder := stream.NewJSONPayloadBuilder(true)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		payloadBuilder.Build(series)
	}
}

func BenchmarkJSONPayloadBuilderThroughputPoints0(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 0, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints2(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 2, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints5(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 5, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 10, 100, 1)
}
func BenchmarkJSONPayloadBuilderThroughputPoints100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 100, 100, 1)
}

func BenchmarkJSONPayloadBuilderTags0(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 0)
}
func BenchmarkJSONPayloadBuilderTags1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderTags2(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 2)
}
func BenchmarkJSONPayloadBuilderTags5(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 5)
}
func BenchmarkJSONPayloadBuilderTags10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 10)
}
func BenchmarkJSONPayloadBuilderTags100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 100)
}

func BenchmarkJSONPayloadBuilderItems1(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 1, 1)
}
func BenchmarkJSONPayloadBuilderItems10(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 10, 1)
}
func BenchmarkJSONPayloadBuilderItems100(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100, 1)
}
func BenchmarkJSONPayloadBuilderItems1000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 1000, 1)
}
func BenchmarkJSONPayloadBuilderItems10000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 10000, 1)
}
func BenchmarkJSONPayloadBuilderItems100000(b *testing.B) {
	benchmarkJSONPayloadBuilderUsage(b, 1, 100000, 1)
}
