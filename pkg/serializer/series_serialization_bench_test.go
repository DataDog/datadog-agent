// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//+build zlib

package serializer

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

func generateData(points int, items int, tags int) metrics.Series {
	series := metrics.Series{}
	for i := 0; i < items; i++ {
		series = append(series, &metrics.Serie{
			Points: func() []metrics.Point {
				ps := make([]metrics.Point, points)
				for p := 0; p < points; p++ {
					ps[p] = metrics.Point{Ts: float64(p*i) * 1.2, Value: float64(p + i)}
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
					ts[t] = fmt.Sprintf("tag%d:%d", t, i*13)
				}
				return ts
			}(),
		})
	}
	return series
}

var payloads forwarder.Payloads
var bufCon = marshaler.DefaultBufferContext()

func serializeSeries(series metrics.Series) forwarder.Payloads {
	pl, err := series.MarshalSplitCompress(bufCon)
	if err != nil {
		panic(err)
	}

	return pl
}

func benchmarkSeriesSerialization(b *testing.B, points int, items int, tags int) {
	series := generateData(points, items, tags)

	b.ResetTimer()
	b.ReportAllocs()

	var payloadCount int
	var payloadCompressedSize uint64
	for n := 0; n < b.N; n++ {
		payloads = serializeSeries(series)
		payloadCount += len(payloads)
		for _, pl := range payloads {
			payloadCompressedSize += uint64(len(*pl))
		}
	}
	b.ReportMetric(float64(payloadCount)/float64(b.N), "payloads")
	b.ReportMetric(float64(payloadCompressedSize)/float64(b.N), "compressed-payload-bytes")
}

/*
ITEMS_K = [1, 5, 10, 50, 100]
POINTS = [2]
TAGS = [10, 50]

def main():
    for items_k in ITEMS_K:
        for points in POINTS:
            for tags in TAGS:
                items = items_k * 1000
                print("func BenchmarkSeries_{items_k:0>3}KItems_{points}Points_{tags:0>2}Tags(b *testing.B) {{ benchmarkSeriesSerialization(b, {points}, {items}, {tags}) }}".format(**locals()))

main()
*/

func BenchmarkSeries_001KItems_2Points_10Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 1000, 10)
}
func BenchmarkSeries_001KItems_2Points_50Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 1000, 50)
}
func BenchmarkSeries_005KItems_2Points_10Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 5000, 10)
}
func BenchmarkSeries_005KItems_2Points_50Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 5000, 50)
}
func BenchmarkSeries_010KItems_2Points_10Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 10000, 10)
}
func BenchmarkSeries_010KItems_2Points_50Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 10000, 50)
}
func BenchmarkSeries_050KItems_2Points_10Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 50000, 10)
}
func BenchmarkSeries_050KItems_2Points_50Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 50000, 50)
}
func BenchmarkSeries_100KItems_2Points_10Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 100000, 10)
}
func BenchmarkSeries_100KItems_2Points_50Tags(b *testing.B) {
	benchmarkSeriesSerialization(b, 2, 100000, 50)
}
