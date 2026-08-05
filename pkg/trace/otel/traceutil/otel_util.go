// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package traceutil provides utilities for converting OTel semantics to DD semantics.
//
// The implementations live in pkg/trace/transform; this package re-exports them
// so that existing callers (e.g. pkg/trace/otel/stats) continue to work without
// taking a direct dependency on the parent pkg/trace module.
package traceutil

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"

	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// Re-exported variables.
var (
	// SignalTypeSet is the OTel attribute set for traces.
	SignalTypeSet = attribute.NewSet(attribute.String("signal", "traces"))
)

// DefaultOTLPServiceName is the default service name for OTel spans when no service name is found in the resource attributes.
const DefaultOTLPServiceName = transform.DefaultOTLPServiceName

// IndexOTelSpans re-exports transform.IndexOTelSpans.
var IndexOTelSpans = transform.IndexOTelSpans

// GetTopLevelOTelSpans re-exports transform.GetTopLevelOTelSpans.
var GetTopLevelOTelSpans = transform.GetTopLevelOTelSpans

// GetOTelAttrVal re-exports transform.GetOTelAttrVal.
var GetOTelAttrVal = transform.GetOTelAttrVal

// GetOTelAttrFromEitherMap re-exports transform.GetOTelAttrFromEitherMap.
var GetOTelAttrFromEitherMap = transform.GetOTelAttrFromEitherMap

// GetOTelAttrValInResAndSpanAttrs re-exports transform.GetOTelAttrValInResAndSpanAttrs.
var GetOTelAttrValInResAndSpanAttrs = transform.GetOTelAttrValInResAndSpanAttrs

// LookupSemanticString re-exports transform.LookupSemanticString.
var LookupSemanticString = transform.LookupSemanticString

// LookupSemanticStringFromDualMaps re-exports transform.LookupSemanticStringFromDualMaps.
var LookupSemanticStringFromDualMaps = transform.LookupSemanticStringFromDualMaps

// SpanKind2Type re-exports transform.SpanKind2Type.
var SpanKind2Type = transform.SpanKind2Type

// GetOTelSpanType re-exports transform.GetOTelSpanType.
var GetOTelSpanType = transform.GetOTelSpanType

// GetOTelService re-exports transform.GetOTelService.
var GetOTelService = transform.GetOTelService

// GetOTelResourceV1 re-exports transform.GetOTelResourceV1.
var GetOTelResourceV1 = transform.GetOTelResourceV1

// GetOTelResourceV2 re-exports transform.GetOTelResourceV2.
var GetOTelResourceV2 = transform.GetOTelResourceV2

// GetOTelOperationNameV2 re-exports transform.GetOTelOperationNameV2.
var GetOTelOperationNameV2 = transform.GetOTelOperationNameV2

// GetOTelContainerTags re-exports transform.GetOTelContainerTags.
var GetOTelContainerTags = transform.GetOTelContainerTags

// OTelTraceIDToUint64 re-exports transform.OTelTraceIDToUint64.
var OTelTraceIDToUint64 = transform.OTelTraceIDToUint64

// OTelSpanIDToUint64 re-exports transform.OTelSpanIDToUint64.
var OTelSpanIDToUint64 = transform.OTelSpanIDToUint64

// OTelSpanKindName re-exports transform.OTelSpanKindName.
var OTelSpanKindName = transform.OTelSpanKindName

// Generic functions cannot be re-exported as vars, so we wrap them.

// LookupSemanticStringWithAccessor wraps transform.LookupSemanticStringWithAccessor.
func LookupSemanticStringWithAccessor[A semantics.Accessor](accessor A, concept semantics.Concept, shouldNormalize bool) string {
	return transform.LookupSemanticStringWithAccessor(accessor, concept, shouldNormalize)
}

// LookupSemanticInt64 re-exports transform.LookupSemanticInt64.
var LookupSemanticInt64 = transform.LookupSemanticInt64

// LookupSemanticFloat64 re-exports transform.LookupSemanticFloat64.
var LookupSemanticFloat64 = transform.LookupSemanticFloat64

// GetOTelSpanTypeWithAccessor wraps transform.GetOTelSpanTypeWithAccessor.
func GetOTelSpanTypeWithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	return transform.GetOTelSpanTypeWithAccessor(span, accessor)
}

// GetOTelServiceWithAccessor wraps transform.GetOTelServiceWithAccessor.
func GetOTelServiceWithAccessor[A semantics.Accessor](accessor A, normalize bool) string {
	return transform.GetOTelServiceWithAccessor(accessor, normalize)
}

// GetOTelResourceV2WithAccessor wraps transform.GetOTelResourceV2WithAccessor.
func GetOTelResourceV2WithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	return transform.GetOTelResourceV2WithAccessor(span, accessor)
}

// GetOTelOperationNameV2WithAccessor wraps transform.GetOTelOperationNameV2WithAccessor.
func GetOTelOperationNameV2WithAccessor[A semantics.Accessor](span ptrace.Span, accessor A) string {
	return transform.GetOTelOperationNameV2WithAccessor(span, accessor)
}

// GetOTelOperationNameV1 re-exports transform.GetOTelOperationNameV1.
var GetOTelOperationNameV1 = transform.GetOTelOperationNameV1

// Ensure unused imports are referenced.
var _ pcommon.Map
