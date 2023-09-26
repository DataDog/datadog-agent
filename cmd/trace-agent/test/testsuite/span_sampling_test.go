// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestSpanSampling(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("one-sampled", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(1, &testutil.TraceConfig{
			MinSpans: 3,
			Keep:     true,
		}, nil)
		singleSpan := p[0][1]
		singleSpan.Metrics["_dd.span_sampling.mechanism"] = 8
		for _, s := range p[0] {
			s.Metrics["_sampling_priority_v1"] = -1
		}

		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			require.Len(t, v.TracerPayloads, 1)
			require.Len(t, v.TracerPayloads[0].Chunks, 1)
			assert.False(t, v.TracerPayloads[0].Chunks[0].DroppedTrace)
			assert.Equal(t, int32(2), v.TracerPayloads[0].Chunks[0].Priority)
			require.Len(t, v.TracerPayloads[0].Chunks[0].Spans, 1)
			assert.Equal(t, 8.0, v.TracerPayloads[0].Chunks[0].Spans[0].Metrics["_dd.span_sampling.mechanism"])
		})
	})
}
