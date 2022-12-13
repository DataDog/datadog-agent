// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package inferredspan

import (
	"crypto/rand"
	"math"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	if input == nil {
		return nil
	}

	output := make(map[string]string)
	for k, v := range input {
		output[k] = v
	}

	// filter out DD_TAGS & DD_EXTRA_TAGS
	ddTags := config.GetConfiguredTags(false)
	for _, tag := range ddTags {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) != 2 {
			log.Warnf("Cannot split tag %s", tag)
			continue
		}
		tagKey := tagParts[0]
		delete(output, tagKey)
	}

	// filter out function specific tags
	for _, tagKey := range functionTagsToIgnore {
		delete(output, tagKey)
	}

	return output
}

// CompleteInferredSpan finishes the inferred span and passes it
// as an API payload to be processed by the trace agent
func (inferredSpan *InferredSpan) CompleteInferredSpan(
	processTrace func(p *api.Payload),
	endTime time.Time,
	isError bool,
	traceID uint64,
	samplingPriority sampler.SamplingPriority) {

	durationIsSet := inferredSpan.Span.Duration != 0
	if inferredSpan.IsAsync {
		// SNSSQS span duration is set in invocationlifecycle/init.go
		if !durationIsSet {
			inferredSpan.Span.Duration = inferredSpan.CurrentInvocationStartTime.UnixNano() - inferredSpan.Span.Start
		}
	} else {
		inferredSpan.Span.Duration = endTime.UnixNano() - inferredSpan.Span.Start
	}
	if isError {
		inferredSpan.Span.Error = 1
	}

	inferredSpan.Span.TraceID = traceID

	traceChunk := &pb.TraceChunk{
		Origin:   "lambda",
		Spans:    []*pb.Span{inferredSpan.Span},
		Priority: int32(samplingPriority),
	}

	tracerPayload := &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{traceChunk},
	}

	processTrace(&api.Payload{
		Source:        info.NewReceiverStats().GetTagStats(info.Tags{}),
		TracerPayload: tracerPayload,
	})
}

// GenerateSpanId creates a secure random span id in specific scenarios, otherwise return a pseudo random id
func GenerateSpanId() uint64 {
	isSnapStart := os.Getenv(tags.InitType) == tags.SnapStartValue
	if isSnapStart {
		max := new(big.Int).SetUint64(math.MaxUint64)
		if randId, err := rand.Int(rand.Reader, max); err != nil {
			log.Debugf("Failed to generate a secure random span id: %v", err)
		} else {
			return randId.Uint64()
		}
	}
	return random.Random.Uint64()
}

// generateInferredSpan declares and initializes a new inferred span
// with the SpanID and TraceID
func (inferredSpan *InferredSpan) generateInferredSpan(startTime time.Time) {

	inferredSpan.CurrentInvocationStartTime = startTime
	inferredSpan.Span = &pb.Span{
		SpanID: GenerateSpanId(),
	}
	log.Debugf("Generated new Inferred span: %+v", inferredSpan)
}

// IsInferredSpansEnabled is used to determine if we need to
// generate and enrich inferred spans for a particular invocation
func IsInferredSpansEnabled() bool {
	return config.Datadog.GetBool("serverless.trace_enabled") && config.Datadog.GetBool("serverless.trace_managed_services")
}

// AddTagToInferredSpan is used to add new tags to the inferred span in
// inferredSpan.Span.Meta[]. Should be used before completing an inferred span.
func (inferredSpan *InferredSpan) AddTagToInferredSpan(key string, value string) {
	inferredSpan.Span.Meta[key] = value
}
