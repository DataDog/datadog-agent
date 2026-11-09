// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/writer"

	"github.com/stretchr/testify/assert"
)

// TestPayloadChunking creates a payload that is several times writer.MaxPayloadSize
// (measured in the legacy wire format) and asserts that the trace-agent forwards
// every trace chunk without dropping any, while never emitting an individual
// output payload larger than writer.MaxPayloadSize.
//
// With the convert-traces feature (enabled by default) the agent re-encodes
// traces in the string-indexed idx format before writing them. That format
// deduplicates repeated strings, so this payload of identical traces serializes
// far smaller than its legacy size and may be emitted as a single payload rather
// than the N+1 the legacy writer produced. The exact number of output payloads is
// therefore encoding-dependent; we drain every payload that arrives and assert on
// the total chunk count and the per-payload size bound rather than a fixed payload
// count.
func TestPayloadChunking(t *testing.T) {
	r := test.Runner{}
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

	trace, err := jsonTraceFromPath("./testdata/trace_with_rates.json")
	if err != nil {
		t.Fatal(err)
	}
	payloadCount := 3
	traceSize := (&pb.TraceChunk{Spans: trace}).Msgsize()
	// make a payload that will cover payloadCount
	var traces pb.Traces
	for size := 0; size < writer.MaxPayloadSize*payloadCount; size += traceSize {
		traces = append(traces, trace)
	}

	if err := r.Post(traces); err != nil {
		t.Fatal(err)
	}
	var got, payloads int
	for got < len(traces) {
		select {
		case p := <-r.Out():
			v, ok := p.(*pb.AgentPayload)
			if !ok {
				t.Fatalf("invalid payload type: %T", p)
			}
			payloads++
			// The trace-agent must never emit a single payload larger than the
			// API's maximum, regardless of how large the incoming chunk was.
			// v.SizeVT() is the uncompressed serialized size, matching what the
			// writer measures before compression.
			assert.LessOrEqualf(t, v.SizeVT(), writer.MaxPayloadSize,
				"payload %d exceeds MaxPayloadSize (%d > %d bytes)", payloads, v.SizeVT(), writer.MaxPayloadSize)
			for _, tracerPayload := range v.IdxTracerPayloads {
				got += len(tracerPayload.Chunks)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for payloads, got %d/%d chunks across %d payloads", got, len(traces), payloads)
		}
	}
	assert.Equal(t, len(traces), got)
	t.Logf("received %d chunks across %d payloads", got, payloads)
}
