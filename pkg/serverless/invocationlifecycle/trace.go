// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

// executionStartInfo is saved information from when an execution span was started
type executionStartInfo struct {
	startTime time.Time
	traceID   uint64
	spanID    uint64
}

// currentExecutionInfo represents information from the start of the current execution span
var currentExecutionInfo executionStartInfo

// startExecutionSpan records information from the start of the invocation.
// It should be called at the start of the invocation.
func startExecutionSpan(startTime time.Time) {
	currentExecutionInfo.startTime = startTime
	currentExecutionInfo.traceID = random.Uint64()
	currentExecutionInfo.spanID = random.Uint64()
}

// endExecutionSpan builds the function execution span and sends it to the intake.
// It should be called at the end of the invocation.
func endExecutionSpan(processTrace func(p *api.Payload), endTime time.Time) {
	duration := endTime.UnixNano() - currentExecutionInfo.startTime.UnixNano()

	executionSpan := &pb.Span{
		Service:  "aws.lambda", // will be replaced by the span processor
		Name:     "aws.lambda",
		Resource: os.Getenv(functionNameEnvVar),
		Type:     "serverless",
		TraceID:  currentExecutionInfo.traceID,
		SpanID:   currentExecutionInfo.spanID,
		Start:    currentExecutionInfo.startTime.UnixNano(),
		Duration: duration,
	}

	traceChunk := &pb.TraceChunk{
		Spans: []*pb.Span{executionSpan},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	processTrace(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
