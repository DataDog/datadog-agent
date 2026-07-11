// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ir

//go:generate go run golang.org/x/tools/cmd/stringer -type=ProbeKind -linecomment -output probe_kind_string.go

// ProbeKind is the kind of probe.
type ProbeKind uint8

const (
	_ ProbeKind = iota

	// ProbeKindLog is a probe that emits a log.
	ProbeKindLog
	// ProbeKindSpan is a probe that emits a span.
	ProbeKindSpan
	// ProbeKindMetric is a probe that updates a metric.
	ProbeKindMetric
	// ProbeKindSnapshot is a probe that emits a snapshot.
	//
	// Internally in rcjson these are log probes with capture_snapshot set to
	// true.
	ProbeKindSnapshot
	// ProbeKindCaptureExpression is a probe that captures specific expressions.
	//
	// Internally in rcjson these are log probes with captureSnapshot=false and
	// captureExpressions set.
	ProbeKindCaptureExpression
	// ProbeKindRuntimeRecovery is an internal probe attached to
	// runtime.recovery. It runs the standard stack machine with a
	// synthesised @exception capture expression bookended by
	// PanicUnwindPrepareOp / PanicUnwindEvictSlotsOp, which validate
	// the recovered panic, compute the unwound stack-depth range
	// (lo, hi], emit a single synthetic event carrying the panic value,
	// and zero every in_progress_calls slot in that range. Synthesised
	// by irgen when the program has at least one function-targeted user
	// probe; never sourced from rcjson.
	ProbeKindRuntimeRecovery

	maxProbeKind uint8 = iota
)

// RuntimeRecoveryProbeID is the well-known synthetic ID assigned to the
// internal probe of kind ProbeKindRuntimeRecovery (see
// irgen.maybeAddRuntimeRecoveryProbe). Exported here so the actuator
// can recognise the probe's identity from a ProbeDefinition.GetID()
// result without depending on irgen.
const RuntimeRecoveryProbeID = "__runtime_recovery__"

// IsValid returns true if the probe kind is valid.
func (k ProbeKind) IsValid() bool {
	return k > 0 && uint8(k) < maxProbeKind
}
