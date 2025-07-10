// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package trace

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// tagFunctionTags is a TracerPayload tag containing the FunctionTags for
	// all of the traces.
	tagFunctionTags = "_dd.tags.function"
)

type tracerPayloadModifier struct {
	functionTags string
}

func (t *tracerPayloadModifier) Modify(tp *pb.TracerPayload) {
	// NOTE: our backend stats computation expects to find these function tags,
	// and more importantly the host group, i.e. primary tags, as attributes in
	// the root span meta. These tags are already being injected into all of
	// the spans in our serverless traces, so we do not need to do anything
	// about that for now. Eventually the stats computation will either move to
	// the tracer and agent or use trace tag on the backend, so so we will not
	// need to worry about it.
	t.ensureFunctionTags(tp)
}

// FunctionTags are applied to traces coming from serverless environments. They
// are included in the trace payload under the `_dd.tags.function` tag and then
// processed into trace tags for downstream systems as part of trace intake.
func (t *tracerPayloadModifier) ensureFunctionTags(tp *pb.TracerPayload) {
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
