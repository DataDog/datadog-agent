// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides utilities for creating and submitting traces in serverless-init
package trace

import (
	"math/rand"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitSpan creates a new span with the provided metadata
func InitSpan(service, name, resource, spanType string, startTime int64, tags map[string]string) *pb.Span {
	traceID := rand.Uint64()
	spanID := rand.Uint64()

	span := &pb.Span{
		Service:  service,
		Name:     name,
		Resource: resource,
		Type:     spanType,
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: 0, // Top-level span
		Start:    startTime,
		Meta:     tags,
	}

	return span
}

// Processor is an interface for processing trace payloads
//
// The payload is a v1 (string-indexed) payload, matching the format the
// trace-agent processes when the convert-traces feature is enabled (the
// default): internally-generated spans are converted with api.ConvertToIdx
// before submission.
type Processor interface {
	Process(*api.PayloadV1)
}

// SubmitSpan submits a completed span to the trace agent.
//
// TODO(v1): the span is built in the legacy pb format and converted here
// because it is shared mutable state (the span modifier and job-span
// completion logic still operate on pb spans). Once the legacy
// (disable-convert-traces) processing path is removed, this should construct
// and accept v1 (idx) spans/chunks directly and drop the conversion.
func SubmitSpan(span *pb.Span, origin string, traceAgent Processor) {
	if span == nil || traceAgent == nil {
		return
	}

	log.Debugf("Submitting inferred span: Service=%s, Name=%s, TraceID=%d, SpanID=%d, Duration=%dns",
		span.Service, span.Name, span.TraceID, span.SpanID, span.Duration)

	traceChunk := &pb.TraceChunk{
		Origin:   origin,
		Priority: int32(sampler.PriorityUserKeep),
		Spans:    []*pb.Span{span},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	// Convert the inferred span to the v1 (string-indexed) payload format so
	// it is processed by the same v1 path used for receiver-ingested traces
	// (the default convert-traces behavior). An empty originPayloadVersion
	// skips the _dd.convertedv1 debug attribute, since this payload was
	// generated in-process, not converted from an incoming wire format.
	traceAgent.Process(&api.PayloadV1{
		Source:        info.NewReceiverStats(true).GetTagStats(info.Tags{}),
		TracerPayload: api.ConvertToIdx(tracerPayload, ""),
	})
	log.Debug("Inferred span submitted successfully")
}
