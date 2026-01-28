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
type Processor interface {
	Process(*api.Payload)
}

// SubmitSpan submits a completed span to the trace agent
func SubmitSpan(span *pb.Span, origin string, traceAgent Processor) {
	if span == nil || traceAgent == nil {
		return
	}

	log.Debugf("Submitting inferred span: Service=%s, Name=%s, TraceID=%d, SpanID=%d, Duration=%dns",
		span.Service, span.Name, span.TraceID, span.SpanID, span.Duration)

	traceChunk := &pb.TraceChunk{
		Origin:   origin,
		Priority: int32(1),
		Spans:    []*pb.Span{span},
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	traceAgent.Process(&api.Payload{
		Source:        info.NewReceiverStats(true).GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
	log.Debug("Inferred span submitted successfully")
}
