// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpsec_test

import (
	"bytes"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycleSubProcessor(t *testing.T) {
	test := func(t *testing.T, appsecEnabled bool) {
		t.Setenv("DD_SERVERLESS_APPSEC_ENABLED", strconv.FormatBool(appsecEnabled))
		asm, _, err := appsec.New()
		if err != nil {
			t.Skipf("appsec disabled: %v", err)
		}

		var sp invocationlifecycle.InvocationSubProcessor
		if appsecEnabled {
			require.NotNil(t, asm)
			sp = asm
		} else {
			require.Nil(t, asm)
		}

		var tracedPayload *api.Payload
		testProcessor := &invocationlifecycle.LifecycleProcessor{
			DetectLambdaLibrary: func() bool { return false },
			ProcessTrace: func(payload *api.Payload) {
				tracedPayload = payload
			},
			SubProcessor: sp,
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

			if appsecEnabled {
				require.Contains(t, tags, "_dd.appsec.json")
				require.Equal(t, int32(sampler.PriorityUserKeep), tracedPayload.TracerPayload.Chunks[0].Priority)
				require.Equal(t, 1.0, tracedPayload.TracerPayload.Chunks[0].Spans[0].Metrics["_dd.appsec.enabled"])
			}
		})
	}

	t.Run("appsec-enabled", func(t *testing.T) {
		test(t, true)
	})

	t.Run("appsec-disabled", func(t *testing.T) {
		test(t, false)
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

func TestInvocationSubProcessorNilInterface(t *testing.T) {
	lp := &invocationlifecycle.LifecycleProcessor{
		DetectLambdaLibrary: func() bool { return true },
		SubProcessor:        (*httpsec.InvocationSubProcessor)(nil),
	}

	assert.True(t, lp.SubProcessor != nil)

	lp.OnInvokeStart(&invocationlifecycle.InvocationStartDetails{
		InvokeEventRawPayload: []byte(
			`{"requestcontext":{"stage":"purple"},"httpmethod":"purple","resource":"purple"}`),
	})

	lp.OnInvokeEnd(&invocationlifecycle.InvocationEndDetails{})
}
