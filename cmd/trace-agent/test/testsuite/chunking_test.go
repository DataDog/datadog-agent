// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

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
	expectedPayloadCount := 4
	// make a payload that will need to be chunked into separate payloads
	var traces pb.Traces
	for range 1290 { // We know 1290 traces will fit into 4 payloads out (we do not calculate up front due to the v1 payload size limit reducing the size required before chunking)
		traces = append(traces, trace)
	}

	if err := r.Post(traces); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(3 * time.Second)
	var got int
	for i := 0; i < expectedPayloadCount+1; i++ {
		select {
		case p := <-r.Out():
			if v, ok := p.(*pb.AgentPayload); ok {
				// ok
				for _, tracerPayload := range v.IdxTracerPayloads {
					got += len(tracerPayload.Chunks)
				}
				continue
			}
			t.Fatalf("invalid payload type: %T", p)
		case <-timeout:
			fmt.Printf("Agent log: %s", r.AgentLog())
			t.Fatalf("timed out waiting for payloads, only got %d of %d", got, len(traces))
		}
	}
	assert.Equal(t, len(traces), got)
}
