// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package invocationlifecycle

import (
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
	serviceEnvVar      = "DD_SERVICE"
)

var (
	functionName = os.Getenv(functionNameEnvVar)
	service      = os.Getenv(serviceEnvVar)
)

// executionSpanInfo is the information needed to create a span representing the Lambda function execution
type executionSpanInfo struct {
	startTime time.Time
	traceID   uint64
	spanID    uint64
}

// currentExecutionSpanInfo represents information about the execution span for the current invocation
var currentExecutionSpanInfo executionSpanInfo

// beginExecutionSpan records information from the start of the invocation in the current execution span info
func beginExecutionSpan(daemon *daemon.Daemon, startTime time.Time) {
	currentExecutionSpanInfo.startTime = startTime
	currentExecutionSpanInfo.traceID = random.Uint64()
	currentExecutionSpanInfo.spanID = random.Uint64()
}

// endExecutionSpan uses information from the end of the invocation plus the current execution span info to build
// the function execution span and sends it to the intake.
func endExecutionSpan(daemon *daemon.Daemon, endTime time.Time) {
	duration := endTime.UnixNano() - currentExecutionSpanInfo.startTime.UnixNano()

	executionSpan := &pb.Span{
		Service:  service,
		Name:     "aws.lambda",
		Resource: functionName,
		Type:     "serverless",
		TraceID:  currentExecutionSpanInfo.traceID,
		SpanID:   currentExecutionSpanInfo.spanID,
		Start:    currentExecutionSpanInfo.startTime.UnixNano(),
		Duration: duration,
	}

	traceChunk := &pb.TraceChunk{
		Spans: []*pb.Span{executionSpan},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	log.Debugf("tracerPayload: %s", tracerPayload)

	daemon.TraceAgent.Get().Process(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
