// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/stretchr/testify/assert"
)

func TestStartExecutionSpan(t *testing.T) {
	startTime := time.Now()
	startExecutionSpan(startTime)

	assert.Equal(t, startTime, currentExecutionInfo.startTime)
	assert.NotEqual(t, 0, currentExecutionInfo.traceID)
	assert.NotEqual(t, 0, currentExecutionInfo.spanID)
}
func TestEndExecutionSpan(t *testing.T) {
	defer os.Unsetenv(functionNameEnvVar)
	defer os.Unsetenv(serviceEnvVar)
	os.Setenv(functionNameEnvVar, "TestFunction")
	os.Setenv(serviceEnvVar, "test-service")

	startTime := time.Now()
	startExecutionSpan(startTime)

	duration := 1 * time.Second
	endTime := startTime.Add(duration)

	var tracePayload *api.Payload
	mockProcessTrace := func(payload *api.Payload) {
		tracePayload = payload
	}

	endExecutionSpan(mockProcessTrace, endTime)
	executionSpan := tracePayload.TracerPayload.Chunks[0].Spans[0]
	assert.Equal(t, "aws.lambda", executionSpan.Name)
	assert.Equal(t, "test-service", executionSpan.Service)
	assert.Equal(t, "TestFunction", executionSpan.Resource)
	assert.Equal(t, "serverless", executionSpan.Type)
	assert.Equal(t, currentExecutionInfo.traceID, executionSpan.TraceID)
	assert.Equal(t, currentExecutionInfo.spanID, executionSpan.SpanID)
	assert.Equal(t, int64(startTime.UnixNano()), executionSpan.Start)
	assert.Equal(t, int64(duration.Nanoseconds()), executionSpan.Duration)

}
