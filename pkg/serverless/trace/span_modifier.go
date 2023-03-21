// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
)

type spanModifier struct {
	tags            map[string]string
	lambdaSpanChan  chan<- *pb.Span
	coldStartSpanId uint64
}

// ModifySpan applies extra logic to the given span
func (s *spanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	if span.Service == "aws.lambda" {
		// service name could be incorrectly set to 'aws.lambda' in datadog lambda libraries
		if s.tags["service"] != "" {
			span.Service = s.tags["service"]
		}
		if s.lambdaSpanChan != nil && span.Name != "aws.lambda.cold_start" {
			s.lambdaSpanChan <- span
		}
	}

	if span.Name == "aws.lambda.load" {
		span.ParentID = s.coldStartSpanId
	}

	if inferredspan.CheckIsInferredSpan(span) {
		log.Debug("Detected a managed service span, filtering out function tags")

		// filter out existing function tags inside span metadata
		spanMetadataTags := span.Meta
		if spanMetadataTags != nil {
			spanMetadataTags = inferredspan.FilterFunctionTags(spanMetadataTags)
			span.Meta = spanMetadataTags
		}
	}
}
