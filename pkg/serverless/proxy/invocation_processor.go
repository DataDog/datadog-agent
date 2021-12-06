// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type invocationProcessor interface {
	process(details *invocationDetails)
}

type proxyProcessor struct {
	outChanel chan *api.Payload
}

func (pp *proxyProcessor) process(invocationDetails *invocationDetails) {
	// TODO here is the part where trace/spans will be created as all information is now available
	log.Debug("[proxy] Invocation ready to be processed ------")
	log.Debug("[proxy] Invocation has started at :", invocationDetails.startTime)
	log.Debug("[proxy] Invocation has finished at :", invocationDetails.endTime)
	log.Debug("[proxy] Invocation invokeHeaders is :", invocationDetails.invokeHeaders)
	log.Debug("[proxy] Invocation invokeEvent payload is :", invocationDetails.invokeEventPayload)
	log.Debug("[proxy] Invocation is in error? :", invocationDetails.isError)
	log.Debug("[proxy] ---------------------------------------")

	span := &pb.Span{
		TraceID:  100,
		SpanID:   100,
		Start:    invocationDetails.startTime.UnixNano(),
		Duration: int64(invocationDetails.endTime.Sub(invocationDetails.startTime) * time.Nanosecond),
		Service:  "maxday-test-universal-instrumentation",
		Resource: "maxday-demo-proxy",
		Name:     "aws.lambda",
	}

	apiPayload := &api.Payload{
		TracerPayload: tracerPayloadWithChunk(traceChunkWithSpan(span)),
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
	}

	pp.outChanel <- apiPayload
}

func tracerPayloadWithChunk(chunk *pb.TraceChunk) *pb.TracerPayload {
	return &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{chunk},
	}
}

func traceChunkWithSpan(span *pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{
		Spans:    []*pb.Span{span},
		Priority: int32(sampler.PriorityUserKeep),
	}
}
