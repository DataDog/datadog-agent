// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package telemetry provides the telemetry for fleet components.
package telemetry

import (
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Span is an alias for ddtrace.Span until we phase ddtrace out.
type Span struct{ ddtrace.Span }

// Finish finishes the span with an error.
func (s *Span) Finish(err error) {
	s.Span.Finish(tracer.WithError(err))
}

// SetResourceName sets the resource name of the span.
func (s *Span) SetResourceName(name string) {
	s.Span.SetTag(ext.ResourceName, name)
}
