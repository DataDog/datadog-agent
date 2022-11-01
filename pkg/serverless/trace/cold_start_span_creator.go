// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/serverless/executioncontext"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
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
type ColdStartSpanCreator struct {
	executionContext *executioncontext.ExecutionContext
	traceAgent       *ServerlessTraceAgent
	spanCreated      bool
}

func (c *ColdStartSpanCreator) create(lambdaSpan *pb.Span) {
	if c.spanCreated {
		log.Debugf("[ASTUYVE] bailing because span is already created")
		return
	}
	log.Debugf("[ASTUYVE] creating a coldstart span")
	ecs := c.executionContext.GetCurrentState()
	if ecs.ColdstartDuration == 0 {
		return
	}

	durationNs := ecs.ColdstartDuration * 1000000

	coldStartSpan := &pb.Span{
		Service:  "aws.lambda",
		Name:     "aws.lambda.cold_start",
		Resource: os.Getenv(functionNameEnvVar),
		SpanID:   random.Random.Uint64(),
		TraceID:  lambdaSpan.TraceID,
		ParentID: lambdaSpan.ParentID,
		Start:    lambdaSpan.Start - durationNs,
		Duration: durationNs,
	}

	traceChunk := &pb.TraceChunk{
		Origin:   "lambda",
		Priority: int32(1),
		Spans:    []*pb.Span{coldStartSpan},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}
	log.Debugf("[ASTUYVE] lambda span %v", lambdaSpan)
	log.Debugf("[ASTUYVE] calling process with span %v", coldStartSpan)

	c.spanCreated = true
	c.traceAgent.ta.Process(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}
