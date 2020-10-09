// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package testsuite

import (
	"sort"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/exportable/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
)

type BySpanID []*pb.Span

func (s BySpanID) Len() int           { return len(s) }
func (s BySpanID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s BySpanID) Less(i, j int) bool { return s[i].SpanID < s[j].SpanID }

func TestSublayerCounts(t *testing.T) {
	r := test.Runner{Verbose: true}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()
	if err := r.RunAgent(nil); err != nil {
		t.Fatal(err)
	}
	defer r.KillAgent()

	span1 := testutil.TestSpan()
	span1.SpanID = 1
	span2 := testutil.TestSpan()
	span2.SpanID = 2
	span2.TraceID = span1.TraceID
	span2.ParentID = span1.SpanID
	span3 := testutil.TestSpan()
	span3.SpanID = 3
	span3.TraceID = span1.TraceID
	span3.ParentID = span2.SpanID
	traceutil.SetTopLevel(span1, true)
	payload := pb.Traces{pb.Trace{span1, span2, span3}, pb.Trace{testutil.RandomSpan()}}
	payload[0][0].Metrics[sampler.KeySamplingPriority] = 2
	payload[1][0].Metrics[sampler.KeySamplingPriority] = -1
	if err := r.Post(payload); err != nil {
		t.Fatal(err)
	}
	waitForTrace(t, &r, func(v pb.TracePayload) {
		if n := len(v.Traces); n != 1 {
			t.Fatalf("expected %d traces, got %d", 1, n)
		}
		if n := len(v.Traces[0].Spans); n != len(payload[0]) {
			t.Fatalf("expected %d spans, got %d", len(payload[0]), n)
		}
		sort.Sort(BySpanID(v.Traces[0].Spans))
		span := v.Traces[0].Spans[0]
		if n := span.Metrics["_sublayers.span_count"]; n != float64(len(payload[0])) {
			t.Fatalf(`expected span.Metrics["_sublayers.span_count"] == %d, but was got %f`, len(payload[0]), n)
		}
	})
}

func TestSublayerDurations(t *testing.T) {
	r := test.Runner{Verbose: true}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()
	if err := r.RunAgent(nil); err != nil {
		t.Fatal(err)
	}
	defer r.KillAgent()
	baseStart := agent.Year2000NanosecTS
	span := func(id, parentId uint64, service, spanType string, start, duration int64) *pb.Span {
		return &pb.Span{
			Name:     "foo",
			Resource: "foo",
			TraceID:  1,
			SpanID:   id,
			ParentID: parentId,
			Service:  service,
			Type:     spanType,
			Start:    baseStart + start,
			Duration: duration,
			Metrics:  map[string]float64{},
		}
	}
	payload := pb.Traces{{
		span(1, 0, "web-server", "web", 0, 130),
		span(2, 1, "pg", "db", 10, 50),
		span(3, 1, "render", "web", 80, 30),
		span(4, 2, "pg-read", "db", 20, 30),
		span(5, 1, "redis", "cache", 15, 55),
		span(6, 1, "rpc1", "rpc", 60, 60),
		span(7, 6, "alert", "rpc", 110, 40),
	}}
	if err := r.Post(payload); err != nil {
		t.Fatal(err)
	}
	waitForTrace(t, &r, func(v pb.TracePayload) {
		if n := len(v.Traces); n != len(payload) {
			t.Fatalf("expected %d traces, got %d", len(payload), n)
		}
		if n := len(v.Traces[0].Spans); n != len(payload[0]) {
			t.Fatalf("expected %d spans, got %d", len(payload[0]), n)
		}
		spans := v.Traces[0].Spans
		sort.Sort(BySpanID(spans))
		for _, tt := range []struct {
			spanIndex int
			metric    string
			duration  float64
		}{
			{0, "_sublayers.duration.by_service.sublayer_service:web-server", 15.0},
			{0, "_sublayers.duration.by_service.sublayer_service:pg", 13.0},
			{0, "_sublayers.duration.by_service.sublayer_service:render", 15.0},
			{0, "_sublayers.duration.by_service.sublayer_service:pg-read", 15.0},
			{0, "_sublayers.duration.by_service.sublayer_service:redis", 28.0},
			{0, "_sublayers.duration.by_service.sublayer_service:rpc1", 30.0},
			{0, "_sublayers.duration.by_service.sublayer_service:alert", 35.0},
			{0, "_sublayers.duration.by_type.sublayer_type:cache", 28.0},
			{0, "_sublayers.duration.by_type.sublayer_type:db", 28.0},
			{0, "_sublayers.duration.by_type.sublayer_type:rpc", 65.0},
			{0, "_sublayers.duration.by_type.sublayer_type:web", 30.0},
			{1, "_sublayers.duration.by_service.sublayer_service:pg", 20.0},
			{1, "_sublayers.duration.by_service.sublayer_service:pg-read", 30.0},
			{1, "_sublayers.duration.by_type.sublayer_type:db", 50.0},
			{5, "_sublayers.duration.by_service.sublayer_service:rpc1", 50.0},
			{5, "_sublayers.duration.by_service.sublayer_service:alert", 40.0},
			{5, "_sublayers.duration.by_type.sublayer_type:rpc", 90.0},
		} {
			val, ok := spans[tt.spanIndex].Metrics[tt.metric]
			if !ok {
				t.Errorf(`Expected spans[%d].Metrics to contain the key "%s", but it was not present.`, tt.spanIndex, tt.metric)
			} else if val != tt.duration {
				t.Errorf(`Expected spans[%d].Metrics["%s"] == %f, but got %f.`, tt.spanIndex, tt.metric, tt.duration, val)
			}
		}
	})
}
