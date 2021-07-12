// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build zlib

package serializer

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	"github.com/DataDog/datadog-agent/pkg/serializer/stream"
)

func buildSeries(numberOfSeries int) metrics.Series {
	testSeries := metrics.Series{}
	for i := 0; i < numberOfSeries; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: float64(time.Now().UnixNano()), Value: 1.2 * float64(i)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}
	return testSeries
}

var results forwarder.Payloads

func benchmarkJSONStream(b *testing.B, passes int, sharedBuffers bool, numberOfSeries int) {
	series := buildSeries(numberOfSeries)
	payloadBuilder := stream.NewJSONPayloadBuilder(sharedBuffers)
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for i := 0; i < passes; i++ {
			results, _ = payloadBuilder.Build(series)
		}
	}
}

func benchmarkSplit(b *testing.B, numberOfSeries int) {
	series := buildSeries(numberOfSeries)
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		results, _ = split.Payloads(series, true, split.MarshalJSON)
	}
}

func BenchmarkJSONStream1(b *testing.B)        { benchmarkJSONStream(b, 1, false, 1) }
func BenchmarkJSONStream10(b *testing.B)       { benchmarkJSONStream(b, 1, false, 10) }
func BenchmarkJSONStream100(b *testing.B)      { benchmarkJSONStream(b, 1, false, 100) }
func BenchmarkJSONStream1000(b *testing.B)     { benchmarkJSONStream(b, 1, false, 1000) }
func BenchmarkJSONStream10000(b *testing.B)    { benchmarkJSONStream(b, 1, false, 10000) }
func BenchmarkJSONStream100000(b *testing.B)   { benchmarkJSONStream(b, 1, false, 100000) }
func BenchmarkJSONStream1000000(b *testing.B)  { benchmarkJSONStream(b, 1, false, 1000000) }
func BenchmarkJSONStream10000000(b *testing.B) { benchmarkJSONStream(b, 1, false, 10000000) }

func BenchmarkJSONStreamShared1(b *testing.B)        { benchmarkJSONStream(b, 1, true, 1) }
func BenchmarkJSONStreamShared10(b *testing.B)       { benchmarkJSONStream(b, 1, true, 10) }
func BenchmarkJSONStreamShared100(b *testing.B)      { benchmarkJSONStream(b, 1, true, 100) }
func BenchmarkJSONStreamShared1000(b *testing.B)     { benchmarkJSONStream(b, 1, true, 1000) }
func BenchmarkJSONStreamShared10000(b *testing.B)    { benchmarkJSONStream(b, 1, true, 10000) }
func BenchmarkJSONStreamShared100000(b *testing.B)   { benchmarkJSONStream(b, 1, true, 100000) }
func BenchmarkJSONStreamShared1000000(b *testing.B)  { benchmarkJSONStream(b, 1, true, 1000000) }
func BenchmarkJSONStreamShared10000000(b *testing.B) { benchmarkJSONStream(b, 1, true, 10000000) }

// Large payloads
func BenchmarkJSONStreamUnSharedLarge1(b *testing.B)    { benchmarkJSONStream(b, 1, false, 100000) }
func BenchmarkJSONStreamUnSharedLarge10(b *testing.B)   { benchmarkJSONStream(b, 10, false, 100000) }
func BenchmarkJSONStreamUnSharedLarge100(b *testing.B)  { benchmarkJSONStream(b, 100, false, 100000) }
func BenchmarkJSONStreamUnSharedLarge1000(b *testing.B) { benchmarkJSONStream(b, 1000, false, 100000) }

func BenchmarkJSONStreamSharedLarge1(b *testing.B)    { benchmarkJSONStream(b, 1, true, 100000) }
func BenchmarkJSONStreamSharedLarge10(b *testing.B)   { benchmarkJSONStream(b, 10, true, 100000) }
func BenchmarkJSONStreamSharedLarge100(b *testing.B)  { benchmarkJSONStream(b, 100, true, 100000) }
func BenchmarkJSONStreamSharedLarge1000(b *testing.B) { benchmarkJSONStream(b, 1000, true, 100000) }

// Medium payloads
func BenchmarkJSONStreamUnSharedMed1(b *testing.B)     { benchmarkJSONStream(b, 1, false, 10000) }
func BenchmarkJSONStreamUnSharedMed10(b *testing.B)    { benchmarkJSONStream(b, 10, false, 10000) }
func BenchmarkJSONStreamUnSharedMed100(b *testing.B)   { benchmarkJSONStream(b, 100, false, 10000) }
func BenchmarkJSONStreamUnSharedMed1000(b *testing.B)  { benchmarkJSONStream(b, 1000, false, 10000) }
func BenchmarkJSONStreamUnSharedMed10000(b *testing.B) { benchmarkJSONStream(b, 10000, false, 10000) }

func BenchmarkJSONStreamSharedMed1(b *testing.B)     { benchmarkJSONStream(b, 1, true, 10000) }
func BenchmarkJSONStreamSharedMed10(b *testing.B)    { benchmarkJSONStream(b, 10, true, 10000) }
func BenchmarkJSONStreamSharedMed100(b *testing.B)   { benchmarkJSONStream(b, 100, true, 10000) }
func BenchmarkJSONStreamSharedMed1000(b *testing.B)  { benchmarkJSONStream(b, 1000, true, 10000) }
func BenchmarkJSONStreamSharedMed10000(b *testing.B) { benchmarkJSONStream(b, 10000, true, 10000) }

// Small payloads
func BenchmarkJSONStreamUnSharedSmall1(b *testing.B)     { benchmarkJSONStream(b, 1, false, 100) }
func BenchmarkJSONStreamUnSharedSmall10(b *testing.B)    { benchmarkJSONStream(b, 10, false, 100) }
func BenchmarkJSONStreamUnSharedSmall100(b *testing.B)   { benchmarkJSONStream(b, 100, false, 100) }
func BenchmarkJSONStreamUnSharedSmall1000(b *testing.B)  { benchmarkJSONStream(b, 1000, false, 100) }
func BenchmarkJSONStreamUnSharedSmall10000(b *testing.B) { benchmarkJSONStream(b, 10000, false, 100) }
func BenchmarkJSONStreamSharedSmall1(b *testing.B)       { benchmarkJSONStream(b, 1, true, 100) }
func BenchmarkJSONStreamSharedSmall10(b *testing.B)      { benchmarkJSONStream(b, 10, true, 100) }
func BenchmarkJSONStreamSharedSmall100(b *testing.B)     { benchmarkJSONStream(b, 100, true, 100) }
func BenchmarkJSONStreamSharedSmall1000(b *testing.B)    { benchmarkJSONStream(b, 1000, true, 100) }
func BenchmarkJSONStreamSharedSmall10000(b *testing.B)   { benchmarkJSONStream(b, 10000, true, 100) }

func BenchmarkSplit1(b *testing.B)        { benchmarkSplit(b, 1) }
func BenchmarkSplit10(b *testing.B)       { benchmarkSplit(b, 10) }
func BenchmarkSplit100(b *testing.B)      { benchmarkSplit(b, 100) }
func BenchmarkSplit1000(b *testing.B)     { benchmarkSplit(b, 1000) }
func BenchmarkSplit10000(b *testing.B)    { benchmarkSplit(b, 10000) }
func BenchmarkSplit100000(b *testing.B)   { benchmarkSplit(b, 100000) }
func BenchmarkSplit1000000(b *testing.B)  { benchmarkSplit(b, 1000000) }
func BenchmarkSplit10000000(b *testing.B) { benchmarkSplit(b, 10000000) }
