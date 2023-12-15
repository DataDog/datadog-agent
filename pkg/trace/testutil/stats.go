// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
)

const (
	defaultEnv         = "none"
	defaultHostname    = "hostname"
	defaultContainerID = "container-id"
)

// BucketWithSpans returns a stats bucket populated with spans stats
func BucketWithSpans(spans []*pb.Span) *pb.ClientStatsBucket {
	srb := stats.NewRawBucket(0, 1e9)
	aggKey := stats.PayloadAggregationKey{
		Env:         defaultEnv,
		Hostname:    defaultHostname,
		Version:     "",
		ContainerID: defaultContainerID,
	}
	for _, s := range spans {
		// override version to ensure all buckets will have the same payload key.
		s.Meta["version"] = ""
		srb.HandleSpan(s, 0, true, "", aggKey, true, nil)
	}
	buckets := srb.Export()
	if len(buckets) != 1 {
		panic("All entries must have the same payload key.")
	}
	for _, b := range buckets {
		return b
	}
	return &pb.ClientStatsBucket{}
}

// RandomBucket returns a bucket made from n random spans, useful to run benchmarks and tests
func RandomBucket(n int) *pb.ClientStatsBucket {
	spans := make([]*pb.Span, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, RandomSpan())
	}

	return BucketWithSpans(spans)
}

// StatsPayloadSample returns a populated client stats payload
func StatsPayloadSample() *pb.ClientStatsPayload {
	bucket := func(start, duration uint64) *pb.ClientStatsBucket {
		return &pb.ClientStatsBucket{
			Start:    start,
			Duration: duration,
			Stats: []*pb.ClientGroupedStats{
				{
					Name:     "name",
					Service:  "service",
					Resource: "/asd/r",
					Hits:     2,
					Errors:   440,
					Duration: 123,
				},
			},
		}
	}
	return &pb.ClientStatsPayload{
		Hostname: "h",
		Env:      "env",
		Version:  "1.2",
		Stats: []*pb.ClientStatsBucket{
			bucket(1, 10),
			bucket(500, 100342),
		},
	}
}

// WithStatsClient replaces the global metrics.StatsClient with c. It also returns
// a function for restoring the original client.
func WithStatsClient(c metrics.StatsClient) func() {
	old := metrics.Client
	timing.Stop() // https://github.com/DataDog/datadog-agent/issues/13934
	metrics.Client = c
	return func() { metrics.Client = old }
}
