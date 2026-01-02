// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"slices"

	"go.opentelemetry.io/collector/component"
)

const (
	// Header keys
	ddAPIKey = "dd-api-key"
)

var (
	// Component IDs
	hostprofilerID      = component.MustNewID("hostprofiler")
	otlpReceiverID      = component.MustNewID("otlp")
	otlpHTTPExporterID  = component.MustNewID("otlphttp")
	infraattributesID   = component.MustNewIDWithName("infraattributes", "default")
	resourcedetectionID = component.MustNewID("resourcedetection")
	ddprofilingID       = component.MustNewIDWithName("ddprofiling", "default")
	hpflareID           = component.MustNewIDWithName("hpflare", "default")

	// Component Types
	infraattributesType   = component.MustNewType("infraattributes")
	resourcedetectionType = component.MustNewType("resourcedetection")
)

// hasProcessorType returns true if the processors list contains a processor of the given type.
func hasProcessorType(processors []component.ID, processorType component.Type) bool {
	return slices.ContainsFunc(processors, func(comp component.ID) bool {
		return comp.Type() == processorType
	})
}
