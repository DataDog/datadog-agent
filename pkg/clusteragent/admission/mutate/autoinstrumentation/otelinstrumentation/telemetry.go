// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
)

// The admission_webhooks subsystem and NoDoubleUnderscoreSep are what the rest of the
// admission controller's telemetry uses (see pkg/clusteragent/admission/metrics), so
// this counter lands alongside library_injection_attempts rather than in a namespace of
// its own.
var unresolvableAnnotations = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"admission_webhooks",
	"otel_instrumentation_unresolvable",
	[]string{"reason", "language"},
	"Number of pods whose OpenTelemetry Instrumentation annotation could not be resolved, and which were therefore left uninstrumented.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

// Swap mode drops any custom resource field it cannot express as Datadog configuration.
// Each drop is logged, but a log line on the admission path is not something anyone
// watches, and "my sampler had no effect" is exactly the kind of report this counter
// answers. The tags stay low cardinality: field is a fixed set of names and value is an
// enumerated custom resource value, never a user-supplied string.
var droppedFields = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"admission_webhooks",
	"otel_instrumentation_dropped_fields",
	[]string{"field", "value"},
	"Number of OpenTelemetry Instrumentation configuration fields that could not be translated to Datadog configuration in swap mode.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

// The mode a pod ends up in is decided per language, from a configured default that the
// custom resource can override. Reconstructing that decision by hand means reading a
// custom resource, an annotation upstream wrote, and the Cluster Agent configuration
// together, so it is worth counting: "why is this pod running a Datadog library when the
// custom resource names an OpenTelemetry image" is answered by the source tag alone.
var modeDecisions = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"admission_webhooks",
	"otel_instrumentation_mode",
	[]string{"mode", "source", "language"},
	"Number of language requests resolved to an OpenTelemetry Instrumentation injection mode, by how the mode was decided.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

// Passthrough mode reproduces upstream's pod mutation, but not all of it: the gaps are
// listed at the top of passthrough.go. Each one a custom resource or a pod can actually
// ask for is counted here, because the symptom otherwise is silence — a musl workload
// gets an init container that copied the wrong payload and fails at start, with nothing
// tying that back to an annotation we ignored.
var passthroughUnsupported = telemetryimpl.GetCompatComponent().NewCounterWithOpts(
	"admission_webhooks",
	"otel_instrumentation_passthrough_unsupported",
	[]string{"field", "language"},
	"Number of OpenTelemetry Instrumentation fields or annotations that passthrough mode ignored because it does not support them.",
	telemetry.Options{NoDoubleUnderscoreSep: true},
)

type passthroughTelemetry struct {
	unsupported telemetry.Counter
}

func newPassthroughTelemetry() *passthroughTelemetry {
	return &passthroughTelemetry{unsupported: passthroughUnsupported}
}

// recordUnsupported counts one ignored field.
func (t *passthroughTelemetry) recordUnsupported(field string, language Language) {
	if t == nil || t.unsupported == nil {
		return
	}
	t.unsupported.Inc(field, string(language))
}

type resolverTelemetry struct {
	unresolvable telemetry.Counter
	mode         telemetry.Counter
}

func newResolverTelemetry() *resolverTelemetry {
	return &resolverTelemetry{unresolvable: unresolvableAnnotations, mode: modeDecisions}
}

// recordUnresolvable counts a pod left uninstrumented because its OpenTelemetry
// annotation could not be resolved. Refusing to inject is otherwise invisible: the pod
// is created normally and nothing in the admission response says why it was skipped.
func (t *resolverTelemetry) recordUnresolvable(reason Reason, language Language) {
	if t == nil || t.unresolvable == nil {
		return
	}
	// An empty language means the failure was not specific to one; keep the tag present
	// but explicit so the series stays well-formed.
	tag := string(language)
	if tag == "" {
		tag = "-"
	}
	t.unresolvable.Inc(string(reason), tag)
}

// recordMode counts one language's mode decision.
func (t *resolverTelemetry) recordMode(mode Mode, source ModeSource, language Language) {
	if t == nil || t.mode == nil {
		return
	}
	t.mode.Inc(string(mode), string(source), string(language))
}

type translationTelemetry struct {
	dropped telemetry.Counter
}

func newTranslationTelemetry() *translationTelemetry {
	return &translationTelemetry{dropped: droppedFields}
}

// recordDropped counts one custom resource field left untranslated.
func (t *translationTelemetry) recordDropped(field, value string) {
	if t == nil || t.dropped == nil {
		return
	}
	if value == "" {
		value = "-"
	}
	t.dropped.Inc(field, value)
}
