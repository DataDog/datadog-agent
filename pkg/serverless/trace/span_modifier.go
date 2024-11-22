// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	functionNameEnvVar = "AWS_LAMBDA_FUNCTION_NAME"
	ddOriginTagName    = "_dd.origin"
	ddOriginTagValue   = "lambda"
	funcTagKey         = "_dd.tags.function"
)

type spanModifier struct {
	service        string
	funcTags       string
	lambdaSpanChan chan<- *pb.Span
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId uint64
	ddOrigin        string
}

// ModifySpan applies extra logic to the given span
func (s *spanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	if span.Service == "aws.lambda" {
		// service name could be incorrectly set to 'aws.lambda' in datadog lambda libraries
		if s.service != "" {
			span.Service = s.service
		}
		if s.lambdaSpanChan != nil && span.Name == "aws.lambda" {
			s.lambdaSpanChan <- span
		}
		// add the keys of all tags as the value of a new tag _dd.tags.function
		// these tags will be added by intake to the traced invocation metric
		// note, this tag set includes custom tags added via DD_TAGS
		span.Meta[funcTagKey] = s.funcTags
	}

	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin := span.Meta[ddOriginTagName]; origin == "" {
		traceutil.SetMeta(span, ddOriginTagName, s.ddOrigin)
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

// SetTags sets the tags to be used by the span modifier.
func (s *spanModifier) SetTags(tags map[string]string) {
	s.service = tags["service"]
	s.funcTags = buildFunctionTags(tags)
}

func buildFunctionTags(tags map[string]string) string {
	buf := strings.Builder{}
	var comma bool
	for k := range tags {
		if strings.HasPrefix(k, "git.") || strings.HasPrefix(k, "_dd.") {
			continue
		}
		if comma {
			buf.WriteString(",")
		}
		buf.WriteString(k)
		comma = true
	}
	return buf.String()
}
