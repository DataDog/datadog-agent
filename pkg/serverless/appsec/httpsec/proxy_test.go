// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec_test

import (
	"bytes"
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/go-libddwaf/v4"
	"github.com/stretchr/testify/require"
)

func init() {
	os.Setenv("DD_APPSEC_WAF_TIMEOUT", "1m")
}

func TestProxyLifecycleProcessor(t *testing.T) {
	if _, err := libddwaf.Usable(); err != nil {
		t.Skip("host not supported by appsec", err)
	}

	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	lp, shutdown, err := appsec.NewWithShutdown(nil)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	defer func() { require.NoError(t, shutdown(context.Background())) }()
	require.NotNil(t, lp)

	execCtx := &executioncontext.ExecutionContext{}
	spanModifier := lp.WrapSpanModifier(execCtx, nil)

	// Helper function to run the proxy monitoring function in the expected order when integrated
	runAppSec := func(requestId string, start invocationlifecycle.InvocationStartDetails) *idx.InternalTraceChunk {
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

		// Create string table for the span
		st := idx.NewStringTable()

		// Create the internal span
		span := idx.NewInternalSpan(st, &idx.Span{
			NameRef:     st.Add("aws.lambda"),
			TypeRef:     st.Add("serverless"),
			ResourceRef: st.Add("GET /"),
			ServiceRef:  st.Add("my-service"),
			SpanID:      spanId,
			Start:       uint64(start.StartTime.UnixNano()),
			Duration:    uint64(time.Minute.Nanoseconds()),
		})

		// Set meta tags as attributes
		span.SetAttributeFromString("request_id", requestId)
		span.SetAttributeFromString("http.status_code", "200")

		// Create the chunk
		chunk := &idx.InternalTraceChunk{
			Spans:   []*idx.InternalSpan{span},
			Strings: st,
		}

		spanModifier.ModifySpan(chunk, span)
		return chunk
	}

	t.Run("api-gateway", func(t *testing.T) {
		// First invocation without any attack
		chunk := runAppSec("request 1", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		appsecEnabled, ok := span.GetAttributeAsFloat64("_dd.appsec.enabled")
		require.True(t, ok)
		require.Equal(t, 1.0, appsecEnabled)

		// Second invocation containing attacks
		chunk = runAppSec("request 2", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-appsec.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span = chunk.Spans[0]
		_, ok = span.GetAttributeAsString("_dd.appsec.json")
		require.True(t, ok, "expected _dd.appsec.json attribute to be present")
		require.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
		appsecEnabled, ok = span.GetAttributeAsFloat64("_dd.appsec.enabled")
		require.True(t, ok)
		require.Equal(t, 1.0, appsecEnabled)
	})

	t.Run("api-gateway-kong", func(t *testing.T) {
		// First invocation without any attack
		chunk := runAppSec("request 1", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-kong.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		appsecEnabled, ok := span.GetAttributeAsFloat64("_dd.appsec.enabled")
		require.True(t, ok)
		require.Equal(t, 1.0, appsecEnabled)

		// Second invocation containing attacks
		chunk = runAppSec("request", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-kong-appsec.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span = chunk.Spans[0]
		_, ok = span.GetAttributeAsString("_dd.appsec.json")
		require.True(t, ok, "expected _dd.appsec.json attribute to be present")
		require.Equal(t, int32(sampler.PriorityUserKeep), chunk.Priority)
		appsecEnabled, ok = span.GetAttributeAsFloat64("_dd.appsec.enabled")
		require.True(t, ok)
		require.Equal(t, 1.0, appsecEnabled)
	})

	t.Run("unsupported-event-type", func(t *testing.T) {
		chunk := runAppSec("request", invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("sqs.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		span := chunk.Spans[0]
		unsupportedEventType, ok := span.GetAttributeAsFloat64("_dd.appsec.unsupported_event_type")
		require.True(t, ok)
		require.Equal(t, 1.0, unsupportedEventType)
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
