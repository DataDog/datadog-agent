// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"os"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
*

	ColdStartSpanCreator is needed because the Datadog Agent, when packaged as an extension, can create
	inferred spans (universal instrumentation) or simply pass those spans created by libraries.
	Until all libraries have been updates to utilize Universal Instrumentation, this class
	is necessary to create cold start spans in the span modifier.

*
*/
const (
	service  = "aws.lambda"
	spanName = "aws.lambda.cold_start"
)

var functionName = os.Getenv(functionNameEnvVar)

type ColdStartSpanCreator struct {
	executionContext *executioncontext.ExecutionContext
	traceAgent       *ServerlessTraceAgent
	createSpan       *sync.Once
}

func (c *ColdStartSpanCreator) create(lambdaSpan *pb.Span) {
	// Prevent infinite loop from SpanModifier call
	if lambdaSpan.Name == "aws.lambda.cold_start" {
		return
	}
	ecs := c.executionContext.GetCurrentState()
	if !ecs.Coldstart || ecs.ColdstartDuration == 0 {

		log.Debugf("[ColdStartSpanCreator] Skipping span creation - no duration received")
		return
	}

	// ColdStartDuration is given in milliseconds
	// APM spans are in nanoseconds
	// millis = nanos * 1e6
	durationNs := ecs.ColdstartDuration * 1e6

	coldStartSpan := &pb.Span{
		Service:  service,
		Name:     spanName,
		Resource: functionName,
		SpanID:   inferredspan.GenerateSpanId(),
		TraceID:  lambdaSpan.TraceID,
		ParentID: lambdaSpan.ParentID,
		Start:    lambdaSpan.Start - int64(durationNs),
		Duration: int64(durationNs),
	}

	log.Debugf("[ColdStartSpanCreator] Lambda span %v", lambdaSpan)
	c.createSpan.Do(func() { c.processSpan(coldStartSpan) })
}

func (c *ColdStartSpanCreator) processSpan(coldStartSpan *pb.Span) {
	log.Debugf("[ColdStartSpanCreator] Creating cold start span %v", coldStartSpan)

	traceChunk := &pb.TraceChunk{
		Origin:   "lambda",
		Priority: int32(1),
		Spans:    []*pb.Span{coldStartSpan},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	c.traceAgent.ta.Process(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
