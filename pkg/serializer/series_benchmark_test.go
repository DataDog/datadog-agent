// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test

package serializer

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
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
			Tags: tagset.CompositeTagsFromSlice(func() []string {
				ts := make([]string, tags)
				for t := 0; t < tags; t++ {
					ts[t] = fmt.Sprintf("tag%d:foobar", t)
				}
				return ts
			}()),
		})
	}
	return series
}

var payloads transaction.BytesPayloads

func BenchmarkSeries(b *testing.B) {
	bench := func(points, items, tags int, build func(series metrics.Series) (transaction.BytesPayloads, error)) func(b *testing.B) {
		return func(b *testing.B) {
			series := generateData(points, items, tags)

			b.ResetTimer()
			b.ReportAllocs()

			var payloadCount int
			var payloadCompressedSize uint64
			for n := 0; n < b.N; n++ {
				var err error
				payloads, err = build(series)
				payloadCount += len(payloads)
				for _, pl := range payloads {
					payloadCompressedSize += uint64(pl.Len())
				}
				require.NoError(b, err)
			}
			b.ReportMetric(float64(payloadCount)/float64(b.N), "payloads")
			b.ReportMetric(float64(payloadCompressedSize)/float64(b.N), "compressed-payload-bytes")
		}
	}
	bufferContext := marshaler.NewBufferContext()
	pb := func(series metrics.Series) (transaction.BytesPayloads, error) {
		iterableSeries := metricsserializer.CreateIterableSeries(metricsserializer.CreateSerieSource(series))
		return iterableSeries.MarshalSplitCompress(bufferContext)
	}

	payloadBuilder := stream.NewJSONPayloadBuilder(true)
	json := func(series metrics.Series) (transaction.BytesPayloads, error) {
		iterableSeries := metricsserializer.CreateIterableSeries(metricsserializer.CreateSerieSource(series))
		return payloadBuilder.BuildWithOnErrItemTooBigPolicy(iterableSeries, stream.DropItemOnErrItemTooBig)
	}

	for _, items := range []int{5, 10, 100, 500, 1000, 10000, 100000} {
		b.Run(fmt.Sprintf("%06d-items", items), func(b *testing.B) {
			for _, points := range []int{5, 10} {
				b.Run(fmt.Sprintf("%02d-points", points), func(b *testing.B) {
					for _, tags := range []int{10, 50} {
						b.Run(fmt.Sprintf("%02d-tags", tags), func(b *testing.B) {
							b.Run("pb", bench(items, points, tags, pb))
							b.Run("json", bench(items, points, tags, json))
						})
					}
				})
			}
		})
	}
}
