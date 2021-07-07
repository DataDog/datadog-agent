// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

const (
	defaultEnv         = "none"
	defaultHostname    = "hostname"
	defaultContainerID = "container-id"
)

// BucketWithSpans returns a stats bucket populated with spans stats
func BucketWithSpans(spans []*stats.WeightedSpan) pb.ClientStatsBucket {
	srb := stats.NewRawBucket(0, 1e9)
	for _, s := range spans {
		// override version to ensure all buckets will have the same payload key.
		s.Meta["version"] = ""
		srb.HandleSpan(s, defaultEnv, defaultHostname, defaultContainerID)
	}
	buckets := srb.Export()
	if len(buckets) != 1 {
		panic("All entries must have the same payload key.")
	}
	for _, b := range srb.Export() {
		return b
	}
	return pb.ClientStatsBucket{}
}

// RandomBucket returns a bucket made from n random spans, useful to run benchmarks and tests
func RandomBucket(n int) pb.ClientStatsBucket {
	spans := make([]*stats.WeightedSpan, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, RandomWeightedSpan())
	}

	return BucketWithSpans(spans)
}
