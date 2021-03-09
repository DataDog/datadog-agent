// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

// +build test

package metrics

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
)

func benchmarkSplitPayloadsSketchesSplit(b *testing.B, numPoints int) {
	testSketchSeries := make(SketchSeriesList, numPoints)
	for i := 0; i < numPoints; i++ {
		testSketchSeries[i] = Makeseries(200)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		split.Payloads(testSketchSeries, true, split.Marshal)
	}
}

func benchmarkSplitPayloadsSketchesNew(b *testing.B, numPoints int) {
	testSketchSeries := make(SketchSeriesList, numPoints)
	for i := 0; i < numPoints; i++ {
		testSketchSeries[i] = Makeseries(200)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		testSketchSeries.MarshalSplitCompress(marshaler.DefaultBufferContext())
	}
}

func BenchmarkSplitPayloadsSketches1(b *testing.B)     { benchmarkSplitPayloadsSketchesSplit(b, 1) }
func BenchmarkSplitPayloadsSketches10(b *testing.B)    { benchmarkSplitPayloadsSketchesSplit(b, 10) }
func BenchmarkSplitPayloadsSketches100(b *testing.B)   { benchmarkSplitPayloadsSketchesSplit(b, 100) }
func BenchmarkSplitPayloadsSketches1000(b *testing.B)  { benchmarkSplitPayloadsSketchesSplit(b, 1000) }
func BenchmarkSplitPayloadsSketches10000(b *testing.B) { benchmarkSplitPayloadsSketchesSplit(b, 10000) }

func BenchmarkMarshalSplitCompress1(b *testing.B)     { benchmarkSplitPayloadsSketchesNew(b, 1) }
func BenchmarkMarshalSplitCompress10(b *testing.B)    { benchmarkSplitPayloadsSketchesNew(b, 10) }
func BenchmarkMarshalSplitCompress100(b *testing.B)   { benchmarkSplitPayloadsSketchesNew(b, 100) }
func BenchmarkMarshalSplitCompress1000(b *testing.B)  { benchmarkSplitPayloadsSketchesNew(b, 1000) }
func BenchmarkMarshalSplitCompress10000(b *testing.B) { benchmarkSplitPayloadsSketchesNew(b, 10000) }
