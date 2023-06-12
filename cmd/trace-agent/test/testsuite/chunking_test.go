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

// TestPayloadChunking creates a payload that is N * writer.MaxPayloadSize and
// expects the trace-agent to writer N+1 payloads and not miss any trace.
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
	timeout := time.After(3 * time.Second)
	var got int
	for i := 0; i < payloadCount+1; i++ {
		select {
		case p := <-r.Out():
			if v, ok := p.(*pb.AgentPayload); ok {
				// ok
				for _, tracerPayload := range v.TracerPayloads {
					got += len(tracerPayload.Chunks)
				}
				continue
			}
			t.Fatalf("invalid payload type: %T", p)
		case <-timeout:
			t.Fatal("timed out waiting for payloads")
		}
	}
	assert.Equal(t, got, len(traces))
}
