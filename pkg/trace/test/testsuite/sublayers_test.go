// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package testsuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

func TestSublayers(t *testing.T) {
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
		span := v.Traces[0].Spans[0]
		if n := span.Metrics["_sublayers.span_count"]; n != float64(len(payload[0])) {
			t.Fatalf(`expected span.Metrics["_sublayers.span_count"] == %d, but was got %f`, len(payload[0]), n)
		}
	})
}
