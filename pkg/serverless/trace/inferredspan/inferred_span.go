// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package inferredspan

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
)

const (
	// tagInferredSpanTagSource is the key to the meta tag
	// that lets us know whether this span should inherit its tags.
	// Expected options are "lambda" and "self"
	tagInferredSpanTagSource = "_inferred_span.tag_source"

	// additional function specific tag keys to ignore
	functionVersionTagKey = "function_version"
	coldStartTagKey       = "cold_start"
)

// InferredSpan contains the pb.Span and Async information
// of the inferredSpan for the current invocation
type InferredSpan struct {
	Span    *pb.Span
	IsAsync bool
	// CurrentInvocationStartTime is the start time of the
	// current invocation not he inferred span. It is used
	// for async function calls to calculate the duration.
	CurrentInvocationStartTime time.Time
}

var functionTagsToIgnore = []string{
	tags.FunctionARNKey,
	tags.FunctionNameKey,
	tags.ExecutedVersionKey,
	tags.EnvKey,
	tags.VersionKey,
	tags.ServiceKey,
	tags.RuntimeKey,
	tags.MemorySizeKey,
	tags.ArchitectureKey,
	functionVersionTagKey,
	coldStartTagKey,
}

// CheckIsInferredSpan determines if a span belongs to a managed service or not
// _inferred_span.tag_source = "self" => managed service span
// _inferred_span.tag_source = "lambda" or missing => lambda related span
func CheckIsInferredSpan(span *pb.Span) bool {
	return strings.Compare(span.Meta[tagInferredSpanTagSource], "self") == 0
}

// FilterFunctionTags filters out DD tags & function specific tags
func FilterFunctionTags(input map[string]string) map[string]string {
	panic("not called")
}

// GenerateSpanId creates a secure random span id in specific scenarios, otherwise return a pseudo random id
//
//nolint:revive // TODO(SERV) Fix revive linter
func GenerateSpanId() uint64 {
	panic("not called")
}

// GenerateInferredSpan declares and initializes a new inferred span with a
// SpanID
func (inferredSpan *InferredSpan) GenerateInferredSpan(startTime time.Time) {
	panic("not called")
}

// IsInferredSpansEnabled is used to determine if we need to
// generate and enrich inferred spans for a particular invocation
func IsInferredSpansEnabled() bool {
	return config.Datadog.GetBool("serverless.trace_enabled") && config.Datadog.GetBool("serverless.trace_managed_services")
}

// AddTagToInferredSpan is used to add new tags to the inferred span in
// inferredSpan.Span.Meta[]. Should be used before completing an inferred span.
func (inferredSpan *InferredSpan) AddTagToInferredSpan(key string, value string) {
	panic("not called")
}
