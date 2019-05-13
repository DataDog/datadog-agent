// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

//+build zlib

package serializer

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
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

func benchmarkJSONStream(b *testing.B, numberOfSeries int) {
	series := buildSeries(numberOfSeries)
	payloadBuilder := jsonstream.NewPayloadBuilder()
	b.ResetTimer()

	totalSize := int64(0)

	for n := 0; n < b.N; n++ {
		results, _ = payloadBuilder.Build(series)
		for _, r := range results {
			totalSize += int64(len(*r))
		}
	}
	fmt.Println("Size:", totalSize)
}

func benchmarkSplit(b *testing.B, numberOfSeries int) {
	series := buildSeries(numberOfSeries)
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		results, _ = split.Payloads(series, true, split.MarshalJSON)
	}
}

func BenchmarkJSONStream1(b *testing.B)        { benchmarkJSONStream(b, 1) }
func BenchmarkJSONStream10(b *testing.B)       { benchmarkJSONStream(b, 10) }
func BenchmarkJSONStream100(b *testing.B)      { benchmarkJSONStream(b, 100) }
func BenchmarkJSONStream1000(b *testing.B)     { benchmarkJSONStream(b, 1000) }
func BenchmarkJSONStream10000(b *testing.B)    { benchmarkJSONStream(b, 10000) }
func BenchmarkJSONStream100000(b *testing.B)   { benchmarkJSONStream(b, 100000) }
func BenchmarkJSONStream1000000(b *testing.B)  { benchmarkJSONStream(b, 1000000) }
func BenchmarkJSONStream10000000(b *testing.B) { benchmarkJSONStream(b, 10000000) }

func BenchmarkSplit1(b *testing.B)        { benchmarkSplit(b, 1) }
func BenchmarkSplit10(b *testing.B)       { benchmarkSplit(b, 10) }
func BenchmarkSplit100(b *testing.B)      { benchmarkSplit(b, 100) }
func BenchmarkSplit1000(b *testing.B)     { benchmarkSplit(b, 1000) }
func BenchmarkSplit10000(b *testing.B)    { benchmarkSplit(b, 10000) }
func BenchmarkSplit100000(b *testing.B)   { benchmarkSplit(b, 100000) }
func BenchmarkSplit1000000(b *testing.B)  { benchmarkSplit(b, 1000000) }
func BenchmarkSplit10000000(b *testing.B) { benchmarkSplit(b, 10000000) }
