// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

const defaultEnv = "none"

// BucketWithSpans returns a stats bucket populated with spans stats
func BucketWithSpans(spans []*stats.WeightedSpan) stats.Bucket {
	srb := stats.NewRawBucket(0, 1e9)
	for _, s := range spans {
		srb.HandleSpan(s, defaultEnv)
	}
	return srb.Export()
}

// RandomBucket returns a bucket made from n random spans, useful to run benchmarks and tests
func RandomBucket(n int) stats.Bucket {
	spans := make([]*stats.WeightedSpan, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, RandomWeightedSpan())
	}

	return BucketWithSpans(spans)
}
