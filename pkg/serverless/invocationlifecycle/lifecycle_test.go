// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/trace/api"

	"github.com/stretchr/testify/assert"
)

func TestGenerateEnhancedErrorMetricOnInvocationEnd(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	metricChannel := make(chan []metrics.MetricSample)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }

	endInvocationTime := time.Now()
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: true}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		MetricChannel:       metricChannel,
	}
	go testProcessor.OnInvokeEnd(&endDetails)

	generatedMetrics := <-metricChannel

	assert.Equal(t, generatedMetrics, []metrics.MetricSample{{
		Name:       "aws.lambda.enhanced.errors",
		Value:      1.0,
		Mtype:      metrics.DistributionType,
		Tags:       extraTags.Tags,
		SampleRate: 1,
		Timestamp:  float64(endInvocationTime.UnixNano()),
	}})
}

func TestStartExecutionSpanNoLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	metricChannel := make(chan []metrics.MetricSample)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return false }

	eventPayload := `a5a{"resource":"/users/create","path":"/users/create","httpMethod":"GET","headers":{"Accept":"*/*","Accept-Encoding":"gzip","x-datadog-parent-id":"1480558859903409531","x-datadog-sampling-priority":"1","x-datadog-trace-id":"5736943178450432258"}}0`
	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{StartTime: startInvocationTime, InvokeEventRawPayload: eventPayload}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		MetricChannel:       metricChannel,
	}
	testProcessor.OnInvokeStart(&startDetails)

	assert.NotEqual(t, uint64(0), currentExecutionInfo.spanID)
	assert.Equal(t, uint64(5736943178450432258), currentExecutionInfo.traceID)
	assert.Equal(t, uint64(1480558859903409531), currentExecutionInfo.parentID)
	assert.Equal(t, startInvocationTime, currentExecutionInfo.startTime)
}

func TestStartExecutionSpanWithLambdaLibrary(t *testing.T) {
	t.Skip() // TODO: FIX ME - currentExecutionInfo is shared across tests

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	metricChannel := make(chan []metrics.MetricSample)
	mockProcessTrace := func(*api.Payload) {}
	mockDetectLambdaLibrary := func() bool { return true }

	startInvocationTime := time.Now()
	startDetails := InvocationStartDetails{StartTime: startInvocationTime}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		MetricChannel:       metricChannel,
	}
	testProcessor.OnInvokeStart(&startDetails)

	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
	assert.NotEqual(t, 0, currentExecutionInfo.traceID)
	assert.NotEqual(t, startInvocationTime, currentExecutionInfo.startTime)
}

func TestEndExecutionSpanNoLambdaLibrary(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")

	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	metricChannel := make(chan []metrics.MetricSample)
	mockDetectLambdaLibrary := func() bool { return false }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}

	currentExecutionInfo = executionStartInfo{
		startTime: startInvocationTime,
		traceID:   123,
		spanID:    1,
		parentID:  3,
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		MetricChannel:       metricChannel,
	}
	testProcessor.OnInvokeEnd(&endDetails)

	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "aws.lambda", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.traceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.spanID, executionSpan.SpanID)
	assert.Equal(t, currentExecutionInfo.parentID, executionSpan.ParentID)
	assert.Equal(t, startInvocationTime.UnixNano(), executionSpan.Start)
	assert.Equal(t, duration.Nanoseconds(), executionSpan.Duration)
}

func TestEndExecutionSpanWithLambdaLibrary(t *testing.T) {
	extraTags := &logs.Tags{
		Tags: []string{"functionname:test-function"},
	}
	metricChannel := make(chan []metrics.MetricSample)
	mockDetectLambdaLibrary := func() bool { return true }

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}
	startInvocationTime := time.Now()
	duration := 1 * time.Second
	endInvocationTime := startInvocationTime.Add(duration)
	endDetails := InvocationEndDetails{EndTime: endInvocationTime, IsError: false}

	currentExecutionInfo = executionStartInfo{
		startTime: startInvocationTime,
		traceID:   123,
		spanID:    1,
	}

	testProcessor := LifecycleProcessor{
		ExtraTags:           extraTags,
		ProcessTrace:        mockProcessTrace,
		DetectLambdaLibrary: mockDetectLambdaLibrary,
		MetricChannel:       metricChannel,
	}
	testProcessor.OnInvokeEnd(&endDetails)

	assert.Equal(t, (*api.Payload)(nil), tracePayload)
}
