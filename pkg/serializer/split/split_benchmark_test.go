// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package split

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func benchmarkSplitPayloadsSketchesSplit(b *testing.B, numPoints int) {

	// Uncomment for testing smaller payloads
	// prevMaxPayloadSizeCompressed := maxPayloadSizeCompressed
	// maxPayloadSizeCompressed = 1024
	// defer func() { maxPayloadSizeCompressed = prevMaxPayloadSizeCompressed }()

	// testSketchSeries := make(metrics.SketchSeriesList, numPoints)
	testSketchSeries := make(metrics.SketchSeriesList, numPoints)
	for i := 0; i < numPoints; i++ {
		testSketchSeries[i] = metrics.Makeseries(200)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		Payloads(testSketchSeries, true, Marshal)
	}
}

func benchmarkSplitPayloadsSketchesNew(b *testing.B, numPoints int) {

	// Uncomment for testing smaller payloads
	// prevMaxPayloadSizeCompressed := maxPayloadSizeCompressed
	// maxPayloadSizeCompressed = 1024
	// defer func() { maxPayloadSizeCompressed = prevMaxPayloadSizeCompressed }()

	testSketchSeries := make(metrics.SketchSeriesList, numPoints)
	for i := 0; i < numPoints; i++ {
		testSketchSeries[i] = metrics.Makeseries(200)
	}

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		testSketchSeries.MarshalSplitCompress()
	}
}

func BenchmarkSplitPayloadsSketches1(b *testing.B) { benchmarkSplitPayloadsSketchesSplit(b, 30000) }

func BenchmarkSplitPayloadsSketches2(b *testing.B) { benchmarkSplitPayloadsSketchesSplit(b, 300) }

func BenchmarkSplitPayloadsSketchesNew1(b *testing.B) { benchmarkSplitPayloadsSketchesNew(b, 30000) }

func BenchmarkSplitPayloadsSketchesNew2(b *testing.B) { benchmarkSplitPayloadsSketchesNew(b, 300) }
