// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec_test

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/require"
)

func init() {
	os.Setenv("DD_APPSEC_WAF_TIMEOUT", "1m")
}

func TestProxyLifecycleProcessor(t *testing.T) {
	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	lp, err := appsec.New(nil)
	if err != nil {
		t.Skipf("appsec disabled: %v", err)
	}
	require.NotNil(t, lp)

	execCtx := &executioncontext.ExecutionContext{}
	spanModifier := lp.WrapSpanModifier(execCtx, nil)

	// Helper function to run the proxy monitoring function in the expected order when integrated
	runAppSec := func(requestId string, start invocationlifecycle.InvocationStartDetails) *pb.TraceChunk {
		// Update the execution context with the data AppSec uses: the request id
		execCtx.SetFromInvocation(start.InvokedFunctionARN, requestId)
		// Run OnInvokeStart to mock the runtime API proxy calling it
		lp.OnInvokeStart(&start)
		// Run OnInvokeEnd to mock the runtime API proxy calling it
		lp.OnInvokeEnd(&invocationlifecycle.InvocationEndDetails{
			EndTime: start.StartTime.Add(time.Minute),
			IsError: false,
		})
		// Run the span modifier to mock the trace-agent calling it when receiving a trace from the tracer
		//nolint:revive // TODO(ASM) Fix revive linter
		spanId := rand.Uint64()
		chunk := &pb.TraceChunk{
			Spans: []*pb.Span{
				{
					Name:     "aws.lambda",
					Type:     "serverless",
					Resource: "GET /",
					Service:  "my-service",
					TraceID:  spanId,
					SpanID:   spanId,
					Start:    start.StartTime.Unix(),
					Duration: int64(time.Minute),
					Meta: map[string]string{
						"request_id":       requestId,
						"http.status_code": "200",
					},
					Metrics: map[string]float64{},
				},
			},
		}
		spanModifier(chunk, chunk.Spans[0])
		return chunk
	}

	t.Run("api-gateway", func(t *testing.T) {
		// First invocation without any attack
		chunk := runAppSec("request 1", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		require.Equal(t, 1.0, span.Metrics["_dd.appsec.enabled"])

		// Second invocation containing attacks
		chunk = runAppSec("request 2", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-appsec.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span = chunk.Spans[0]
		require.Contains(t, span.Meta, "_dd.appsec.json")
		require.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
		require.Equal(t, 1.0, span.Metrics["_dd.appsec.enabled"])
	})

	t.Run("api-gateway-kong", func(t *testing.T) {
		// First invocation without any attack
		chunk := runAppSec("request 1", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-kong.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		require.Equal(t, 1.0, span.Metrics["_dd.appsec.enabled"])

		// Second invocation containing attacks
		chunk = runAppSec("request", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-kong-appsec.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span = chunk.Spans[0]
		require.Contains(t, span.Meta, "_dd.appsec.json")
		require.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
		require.Equal(t, 1.0, span.Metrics["_dd.appsec.enabled"])
	})

	t.Run("unsupported-event-type", func(t *testing.T) {
		// First invocation without any attack
		chunk := runAppSec("request", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("sqs.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		require.Equal(t, 1.0, span.Metrics["_dd.appsec.unsupported_event_type"])
	})
}

// Helper function for reading test file
func getEventFromFile(filename string) []byte {
	event, err := os.ReadFile("../../trace/testdata/event_samples/" + filename)
	if err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	buf.WriteString("a5a")
	buf.Write(event)
	buf.WriteString("0")
	return buf.Bytes()
}
