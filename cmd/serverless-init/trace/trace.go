// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides utilities for creating and submitting traces in serverless-init
package trace

import (
	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InitChunk creates a new trace chunk with a single span with the provided metadata
func InitChunk(service, name, resource, spanType string, startTime int64, tags map[string]string) *idx.InternalTraceChunk {
	// Use helper function to create the chunk with a single span
	return idx.NewInternalTraceChunkWithSpan(
		service, name, resource, spanType,
		0, // parentID - top-level span
		startTime,
		tags,
		1,  // priority
		"", // origin (can be set later via SubmitSpan)
	)
}

// Processor is an interface for processing trace payloads
type Processor interface {
	ProcessV1(*api.PayloadV1)
}

// SubmitSpan submits a completed span chunk to the trace agent
func SubmitSpan(chunk *idx.InternalTraceChunk, origin string, traceAgent Processor) {
	if chunk == nil || traceAgent == nil {
		return
	}

	// Log the first span for debugging
	if len(chunk.Spans) > 0 {
		firstSpan := chunk.Spans[0]
		log.Debugf("Submitting inferred span: Service=%s, Name=%s, SpanID=%d, Duration=%dns",
			firstSpan.Service(), firstSpan.Name(), firstSpan.SpanID(), firstSpan.Duration())
	}

	// Set origin on the chunk
	chunk.SetOrigin(origin)

	// Create an InternalTracerPayload with the chunk
	tracerPayload := &idx.InternalTracerPayload{
		Strings: chunk.Strings,
		Chunks:  []*idx.InternalTraceChunk{chunk},
	}

	traceAgent.ProcessV1(&api.PayloadV1{
		Source:        info.NewReceiverStats(true).GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
	log.Debug("Inferred span submitted successfully")
}
