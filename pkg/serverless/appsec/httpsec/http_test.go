// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless
// +build serverless

package httpsec_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/require"
)

func TestLifecycleSubProcessor(t *testing.T) {
	t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", "true")
	asm, err := appsec.New()
	if err != nil {
		t.Skipf("appsec disabled: %v", err)
	}

	var tracedPayload *api.Payload
	testProcessor := &invocationlifecycle.LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return false },
		ProcessTrace: func(payload *api.Payload) {
			tracedPayload = payload
		},
		SubProcessor: httpsec.NewInvocationSubProcessor(asm),
	}

	t.Run("api-gateway", func(t *testing.T) {
		// First invocation without any attack
		testProcessor.OnInvokeStart(&invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		testProcessor.OnInvokeEnd(&invocationlifecycle.InvocationEndDetails{
			EndTime: time.Now(),
			IsError: false,
		})
		tags := testProcessor.GetTags()
		require.Equal(t, "api-gateway", tags["function_trigger.event_source"])
		require.Equal(t, "arn:aws:apigateway:us-east-1::/restapis/1234567890/stages/prod", tags["function_trigger.event_source_arn"])
		require.Equal(t, "POST", tags["http.method"])
		require.Equal(t, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com", tags["http.url"])

		// Second invocation containing attacks
		testProcessor.OnInvokeStart(&invocationlifecycle.InvocationStartDetails{
			InvokeEventRawPayload: getEventFromFile("api-gateway-appsec.json"),
			InvokedFunctionARN:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
		})
		testProcessor.OnInvokeEnd(&invocationlifecycle.InvocationEndDetails{
			EndTime: time.Now(),
			IsError: false,
		})
		tags = testProcessor.GetTags()
		require.Equal(t, "api-gateway", tags["function_trigger.event_source"])
		require.Equal(t, "arn:aws:apigateway:us-east-1::/restapis/1234567890/stages/prod", tags["function_trigger.event_source_arn"])
		require.Equal(t, "POST", tags["http.method"])
		require.Equal(t, "70ixmpl4fl.execute-api.us-east-2.amazonaws.com", tags["http.url"])
		require.Contains(t, tags, "_dd.appsec.json")
		require.Equal(t, int32(sampler.PriorityUserKeep), tracedPayload.TracerPayload.Chunks[0].Priority)
		require.Equal(t, 1.0, tracedPayload.TracerPayload.Chunks[0].Spans[0].Metrics["_dd.appsec.enabled"])
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
