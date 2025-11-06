// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
	ddOriginTagName    = "_dd.origin"
	ddOriginTagValue   = "lambda"
)

type spanModifier struct {
	tags           map[string]string
	lambdaSpanChan chan<- *LambdaSpan
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId uint64
	ddOrigin        string
}

// ModifySpan applies extra logic to the given span
func (s *spanModifier) ModifySpan(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
	if span.Service() == "aws.lambda" {
		// service name could be incorrectly set to 'aws.lambda' in datadog lambda libraries
		if s.tags["service"] != "" {
			span.SetService(s.tags["service"])
		}
		if s.lambdaSpanChan != nil && span.Name() == "aws.lambda" {
			s.lambdaSpanChan <- &LambdaSpan{
				TraceID: chunk.TraceID[:],
				Span:    span,
			}
		}
	}

	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin, ok := span.GetAttributeAsString(ddOriginTagName); !ok || origin == "" {
		span.SetStringAttribute(ddOriginTagName, s.ddOrigin)
	}

	if span.Name() == "aws.lambda.load" {
		span.SetParentID(s.coldStartSpanId)
	}

	if inferredspan.CheckIsInferredSpan(span) {
		log.Debug("Detected a managed service span, filtering out function tags")

		// filter out existing function tags inside span metadata
		inferredspan.FilterFunctionTags(span)
	}
}

// SetTags sets the tags to be used by the span modifier.
func (s *spanModifier) SetTags(tags map[string]string) {
	s.tags = tags
}
