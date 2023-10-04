// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"math/rand"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// SpanConfig defines the configuration for generating spans.
type SpanConfig struct {
	// MinTags specifies the minimum number of tags this span should have.
	MinTags int
	// MaxTags specifies the maximum number of tags this span should have.
	MaxTags int
}

// TraceConfig specifies trace generating configuration.
type TraceConfig struct {
	// MinSpans specifies the minimum number of spans per trace.
	MinSpans int
	// MaxSpans specifies the maximum number of spans per trace.
	MaxSpans int
	// Keep reports whether this trace should be marked as sampling priority
	// "User Keep"
	Keep bool
}

// GeneratePayload generates a new payload.
// The last span of a generated trace is the "root" of that trace
func GeneratePayload(n int, tc *TraceConfig, sc *SpanConfig) pb.Traces {
	if n == 0 {
		return pb.Traces{}
	}
	out := make(pb.Traces, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, GenerateTrace(tc, sc))
	}
	return out
}

// GenerateTrace generates a valid trace using the given config.
func GenerateTrace(tc *TraceConfig, sc *SpanConfig) pb.Trace {
	if tc == nil {
		tc = &TraceConfig{}
	}
	if sc == nil {
		sc = &SpanConfig{}
	}
	if tc.MinSpans == 0 {
		tc.MinSpans = 1
	}
	if tc.MaxSpans < tc.MinSpans {
		tc.MaxSpans = tc.MinSpans
	}
	n := tc.MinSpans
	if tc.MaxSpans > tc.MinSpans {
		n += rand.Intn(tc.MaxSpans - tc.MinSpans)
	}
	t := make(pb.Trace, 0, n)
	var (
		maxd int64
		root *pb.Span
	)
	for i := 0; i < n; i++ {
		s := GenerateSpan(sc)
		if s.Duration > maxd {
			root = s
			maxd = s.Duration
		}
		t = append(t, s)
	}
	if tc.Keep {
		root.Metrics["_sampling_priority_v1"] = 2
	}
	for _, span := range t {
		if span == root {
			continue
		}
		span.TraceID = root.TraceID
		span.ParentID = root.SpanID
		span.Start = root.Start + rand.Int63n(root.Duration-span.Duration)
	}
	return t
}

// GenerateSpan generates a random root span with all fields filled in.
func GenerateSpan(c *SpanConfig) *pb.Span {
	pickString := func(all []string) string { return all[rand.Intn(len(all))] }
	id := uint64(rand.Int63())
	duration := 1 + rand.Int63n(1_000_000_000) // between 1ns and 1s
	s := &pb.Span{
		Service:  pickString(services),
		Name:     pickString(names),
		Resource: pickString(resources),
		TraceID:  id,
		SpanID:   id,
		ParentID: 0,
		Start:    time.Now().UnixNano() - duration,
		Duration: duration,
		Error:    int32(rand.Intn(2)),
		Meta:     make(map[string]string),
		Metrics:  make(map[string]float64),
		Type:     pickString(types),
	}
	ntags := c.MinTags
	if c.MaxTags > c.MinTags {
		ntags += rand.Intn(c.MaxTags - c.MinTags)
	}
	if ntags == 0 {
		// no tags needed
		return s
	}
	nmetrics := 0
	if ntags > 4 {
		// make 25% of tags Metrics when we have more than 4
		nmetrics = ntags / 4
	}
	// ensure we have enough to pick from
	if nmetrics > len(spanMetrics) {
		nmetrics = len(spanMetrics)
	}
	nmeta := ntags - nmetrics
	if nmeta > len(metas) {
		nmeta = len(metas)
	}
	for i := 0; i < nmeta; i++ {
		for k := range metas {
			if _, ok := s.Meta[k]; ok {
				continue
			}
			s.Meta[k] = pickString(metas[k])
			break
		}
	}
	for i := 0; i < nmetrics; i++ {
		for {
			k := pickString(spanMetrics)
			if _, ok := s.Metrics[k]; ok {
				continue
			}
			s.Metrics[k] = rand.Float64()
			break
		}
	}
	return s
}
