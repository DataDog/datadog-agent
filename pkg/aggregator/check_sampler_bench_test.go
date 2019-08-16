// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func benchmarkAddBucket(bucketValue int, b *testing.B) {
	checkSampler := newCheckSampler()

	bucket := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      bucketValue,
		LowerBound: 10.0,
		UpperBound: 20.0,
		Tags:       []string{"foo", "bar"},
		Timestamp:  12345.0,
	}

	for n := 0; n < b.N; n++ {
		checkSampler.addBucket(bucket)
		// reset bucket cache
		checkSampler.lastBucketValue = make(map[ckey.ContextKey]int)
		checkSampler.lastSeenBucket = make(map[ckey.ContextKey]time.Time)
	}
}

func BenchmarkAddBucket1(b *testing.B)        { benchmarkAddBucket(1, b) }
func BenchmarkAddBucket100(b *testing.B)      { benchmarkAddBucket(100, b) }
func BenchmarkAddBucket10000(b *testing.B)    { benchmarkAddBucket(10000, b) }
func BenchmarkAddBucket1000000(b *testing.B)  { benchmarkAddBucket(1000000, b) }
func BenchmarkAddBucket10000000(b *testing.B) { benchmarkAddBucket(10000000, b) }
