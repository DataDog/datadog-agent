// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package modifier provides trace payload modification functionality for serverless environments
package modifier

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// tagFunctionTags is a TracerPayload tag containing the FunctionTags for
	// all of the traces.
	tagFunctionTags = "_dd.tags.function"
)

// TracerPayloadModifier modifies tracer payloads for serverless environments
type TracerPayloadModifier struct {
	functionTags string
}

// NewTracerPayloadModifier creates a new TracerPayloadModifier with the given function tags
func NewTracerPayloadModifier(functionTags string) *TracerPayloadModifier {
	return &TracerPayloadModifier{
		functionTags: functionTags,
	}
}

// Modify updates the tracer payload to include the `_dd.tags.function` tag in
// its tags structure, containing the function tags that need to be applied to
// the payload.
func (t *TracerPayloadModifier) Modify(tp *pb.TracerPayload) {
	// NOTE: our backend stats computation expects to find these function tags,
	// and more importantly the host group, i.e. primary tags, as attributes in
	// the root span meta. These tags are already being injected into all of
	// the spans in our serverless traces, so we do not need to do anything
	// about that for now. Trace computation is now controlled by
	//DD_SERVERLESS_INIT_DISABLE_TRACE_STATS and DD_SERVERLESS_INIT_ENABLE_BACKEND_TRACE_STATS,
	// and can happen in the agent.
	t.ensureFunctionTags(tp)
}

// FunctionTags are applied to traces coming from serverless environments. They
// are included in the trace payload under the `_dd.tags.function` tag and then
// processed into trace tags for downstream systems as part of trace intake.
func (t *TracerPayloadModifier) ensureFunctionTags(tp *pb.TracerPayload) {
	if t.functionTags == "" {
		return
	}

	if tp.Tags == nil {
		tp.Tags = make(map[string]string)
	}

	if existingFunctionTags, ok := tp.Tags[tagFunctionTags]; ok && existingFunctionTags != t.functionTags {
		log.Debugf("The trace payload already has function tags '%v'. Replacing them with '%v'.",
			existingFunctionTags,
			t.functionTags,
		)
	}

	tp.Tags[tagFunctionTags] = t.functionTags
	log.Debugf("set function tags to %v", tp.Tags[tagFunctionTags])
}
