// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

import (
	"cmp"
	"encoding/json"
	"iter"
)

// ProbeIDer is an interface that allows for comparison of probe definitions.
type ProbeIDer interface {
	// GetID returns the ID of the probe.
	GetID() string
	// GetVersion returns the version of the probe.
	GetVersion() int
}

// ProbeDefinition abstracts the configuration of a probe.
type ProbeDefinition interface {
	ProbeIDer
	// GetTags returns the tags of the probe.
	GetTags() []string
	// GetKind returns the kind of the probe.
	GetKind() ProbeKind
	// GetWhere returns the where clause of the probe.
	GetWhere() Where
	// GetCaptureConfig returns the capture configuration of the probe.
	GetCaptureConfig() CaptureConfig
	// ThrottleConfig returns the throttle configuration of the probe.
	GetThrottleConfig() ThrottleConfig
	// GetTemplate returns the template of the probe.
	GetTemplate() TemplateDefinition
	// GetCaptureExpressions returns the capture expressions of the probe, or
	// nil if the probe does not have capture expressions.
	GetCaptureExpressions() []CaptureExpressionDefinition
}

// CaptureExpressionDefinition defines a single capture expression on a probe.
type CaptureExpressionDefinition interface {
	GetName() string
	GetDSL() string
	GetJSON() json.RawMessage
	// GetCaptureConfig returns per-expression capture limits, or nil for probe defaults.
	GetCaptureConfig() CaptureConfig
}

// CompareProbeIDs compares two probe definitions by their ID and version.
func CompareProbeIDs[A, B ProbeIDer](a A, b B) int {
	return cmp.Or(
		cmp.Compare(a.GetID(), b.GetID()),
		cmp.Compare(b.GetVersion(), a.GetVersion()), // reverse version order
	)
}

// TemplateDefinition represents the configuration-time template definition
type TemplateDefinition interface {
	GetTemplateString() string
	GetSegments() iter.Seq[TemplateSegmentDefinition]
}

// TemplateSegmentDefinition represents a configuration-time template segment
type TemplateSegmentDefinition interface {
	TemplateSegment() // marker method
}

// TemplateSegmentString represents a string literal segment in configuration
type TemplateSegmentString interface {
	TemplateSegmentDefinition
	GetString() string
}

// TemplateSegmentExpression represents an expression segment in configuration
type TemplateSegmentExpression interface {
	TemplateSegmentDefinition
	GetDSL() string
	GetJSON() json.RawMessage
}

// Where is a where clause of a probe.
type Where interface {
	Where() // marker method
}

// FunctionWhere is a where clause of a probe that is a function.
type FunctionWhere interface {
	Where
	Location() (functionName string)
}

// LineWhere is a where clause of a probe that is a line within a function.
type LineWhere interface {
	Where
	Line() (functionName string, sourceFile string, lineNumber string)
}

// CaptureConfig is the capture configuration of a probe.
type CaptureConfig interface {
	GetMaxReferenceDepth() uint32
	GetMaxFieldCount() uint32
	GetMaxLength() uint32
	GetMaxCollectionSize() uint32
}

// ThrottleConfig is the throttle configuration of a probe.
type ThrottleConfig interface {
	GetThrottlePeriodMs() uint32
	GetThrottleBudget() int64
}
