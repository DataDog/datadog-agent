// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// UnmarshalProbe unmarshals a Probe from a JSON byte slice.
func UnmarshalProbe(data []byte) (Probe, error) {
	type config struct {
		ID              string `json:"id"`
		Type            Type   `json:"type"`
		CaptureSnapshot bool   `json:"captureSnapshot"`
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("UnmarshalProbe: failed to parse json: %w", err)
	}
	var v Probe
	switch c.Type {
	case TypeDefault,
		TypeLegacyServiceConfig,
		TypeServiceConfig,
		TypeSpanDecorationProbe:
		return nil, fmt.Errorf(
			"UnmarshalProbe: unexpected config.Type: %s", c.Type,
		)
	case TypeLogProbe:
		if c.CaptureSnapshot {
			v = new(SnapshotProbe)
		} else {
			v = new(LogProbe)
		}
	case TypeMetricProbe:
		v = new(MetricProbe)
	case TypeSpanProbe:
		v = new(SpanProbe)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return nil, fmt.Errorf(
			"UnmarshalProbe: id: %s, kind: %v: failed to parse json: %w",
			c.ID, v.GetKind(), err,
		)
	}
	return v, nil
}

// Validate can be used to validate a probe before it is used.
func Validate(p Probe) error {
	return p.validate()
}

// Probe is the interface for all probe types.
type Probe interface {
	ir.ProbeDefinition

	validate() error
}

// ProbeCommon contains fields that are shared by all probe definitions that
// originate from Remote Configuration (RC). These fields map 1-to-1 to the
// JSON representation received from the control plane and therefore should
// remain stable across versions.
//
// The struct purposefully does not implement ir.ProbeDefinition directly. That
// interface is satisfied by dedicated wrapper types such as MetricProbe,
// LogProbe and SnapshotProbe which embed ProbeCommon and complement it with
// additional behaviour.
type ProbeCommon struct {
	// ID is the unique identifier of the probe issued by the control plane.
	ID string `json:"id"`
	// Version is a monotonically increasing version used to invalidate older
	// versions that may still be cached locally.
	Version int `json:"version"`
	// Type is a string that describes the concrete probe kind. This field
	// is used by UnmarshalProbe to decide which concrete Go type to instantiate.
	Type string `json:"type"`
	// Language is the (optional) target language/runtime for which the probe is
	// intended (e.g. "go", "java").
	Language string `json:"language,omitempty"`
	// Where specifies where in the target application the probe should be applied.
	Where *Where `json:"where"`
	// Tags is an (optional) set of arbitrary key/value tags that originate
	// from the RC backend and are propagated to telemetry for correlation.
	Tags []string `json:"tags,omitempty"`
	// EvaluateAt is the (optional) execution point that determines when the probe
	// expression should be evaluated (e.g. "method_end").
	EvaluateAt string `json:"evaluateAt,omitempty"`
}

// GetID returns the ID of the probe.
func (pc *ProbeCommon) GetID() string { return pc.ID }

// GetVersion returns the version of the probe.
func (pc *ProbeCommon) GetVersion() int { return pc.Version }

// GetTags returns the tags of the probe.
func (pc *ProbeCommon) GetTags() []string { return pc.Tags }

// GetWhere returns the where clause of the probe.
func (pc *ProbeCommon) GetWhere() ir.Where { return getWhere(pc.Where) }

// GetEvaluateAt returns the evaluateAt clause of the probe.
func (pc *ProbeCommon) GetEvaluateAt() string { return pc.EvaluateAt }

// Where specifies where in the target application a probe should be applied.
type Where struct {
	// TypeName is the name of the type (e.g. class) where the probe is located.
	TypeName string `json:"typeName,omitempty"`
	// MethodName is the name of the method where the probe is located.
	MethodName string `json:"methodName,omitempty"`
	// SourceFile is the source file where the probe is located.
	SourceFile string `json:"sourceFile,omitempty"`
	// Signature is the signature of the method where the probe is located.
	Signature string `json:"signature,omitempty"`
	// Lines are the line numbers in the source file where the probe is located.
	Lines []string `json:"lines,omitempty"`
}

// When specifies a condition for a probe to be triggered.
type When struct {
	// DSL is a Datadog Expression Language (DSL) expression that must evaluate
	// to true for the probe to be triggered.
	DSL string `json:"dsl"`
	// JSON is a JSON representation of the DSL expression.
	JSON json.RawMessage `json:"json"`
}

// Value specifies a value to be extracted by a probe.
type Value struct {
	// DSL is a Datadog Expression Language (DSL) expression that evaluates to
	// the value to be extracted.
	DSL string `json:"dsl,omitempty"`
	// JSON is a JSON representation of the DSL expression.
	JSON json.RawMessage `json:"json,omitempty"`
}

// Capture specifies how much data to capture from the application.
type Capture struct {
	// MaxReferenceDepth is the maximum depth of nested objects to capture.
	MaxReferenceDepth int `json:"maxReferenceDepth"`
	// MaxFieldCount is the maximum number of fields to capture from an object.
	MaxFieldCount int `json:"maxFieldCount,omitempty"`
	// MaxCollectionSize is the maximum number of elements to capture from a
	// collection.
	MaxCollectionSize int `json:"maxCollectionSize,omitempty"`
}

// Sampling specifies how often to trigger a probe.
type Sampling struct {
	// SnapshotsPerSecond is the maximum number of snapshots to take per second.
	SnapshotsPerSecond float64 `json:"snapshotsPerSecond"`
}

// MetricProbe is a probe that emits a metric.
type MetricProbe struct {
	ProbeCommon
	// Kind is the kind of metric to emit (e.g. "count", "gauge", "histogram").
	Kind string `json:"kind"`
	// MetricName is the name of the metric to emit.
	MetricName string `json:"metricName"`
	// Value is the value of the metric to emit.
	Value *Value `json:"value,omitempty"`
}

func (m *MetricProbe) validate() error {
	return validateWhere(m.Where)
}

// GetCaptureConfig returns the capture configuration of the probe.
func (m *MetricProbe) GetCaptureConfig() ir.CaptureConfig {
	return noCaptureConfig{}
}

// GetThrottleConfig returns the throttle configuration of the probe.
func (m *MetricProbe) GetThrottleConfig() ir.ThrottleConfig {
	return infiniteThrottleConfig{}
}

// GetKind returns the kind of the probe.
func (m *MetricProbe) GetKind() ir.ProbeKind { return ir.ProbeKindMetric }

// LogProbeCommon groups the configuration fields that are shared between
// LogProbe and SnapshotProbe.
//
// The struct exists purely for composition purposes and is not intended to be
// used directly by business logic; instead, the concrete probe types embed it
// and expose richer behaviour via ir.ProbeDefinition.
type LogProbeCommon struct {
	ProbeCommon
	// When specifies a condition for the probe to be triggered.
	When *When `json:"when,omitempty"`
	// Capture specifies how much data to capture from the application.
	Capture *Capture `json:"capture,omitempty"`
	// Sampling specifies how often to trigger the probe.
	Sampling *Sampling `json:"sampling,omitempty"`
	// Message is the message to emit when the probe is triggered.
	Message string `json:"message,omitempty"`
	// Template is the message template of the log to emit.
	Template string `json:"template"`
	// Segments are the segments of the log message template.
	Segments []json.RawMessage `json:"segments"`
}

// GetCaptureConfig returns the capture configuration of the probe.
func (l *LogProbeCommon) GetCaptureConfig() ir.CaptureConfig {
	return (*irCaptureConfig)(l.Capture)
}

// LogProbe is a probe that emits a log.
type LogProbe struct {
	LogProbeCommon
	// CaptureSnapshot is always false for log probes.
	CaptureSnapshot False `json:"captureSnapshot"`
}

func (l *LogProbe) validate() error {
	if err := validateWhere(l.Where); err != nil {
		return err
	}
	if l.Template == "" {
		return errors.New("template must be set")
	}
	if len(l.Segments) == 0 {
		return errors.New("segments must be set")
	}
	return nil
}

// GetThrottleConfig returns the throttle configuration of the probe.
func (l *LogProbe) GetThrottleConfig() ir.ThrottleConfig {
	return (*logThrottleConfig)(l.Sampling)
}

// GetKind returns the kind of the probe.
func (l *LogProbe) GetKind() ir.ProbeKind { return ir.ProbeKindLog }

// SnapshotProbe represents a probe that captures a complete snapshot of the
// local variables and object graph when it is triggered. It behaves similarly
// to a log probe with `captureSnapshot=true`, but it is treated as a distinct
// probe kind so that the downstream instrumentation pipeline can apply
// different handling (e.g. throttling, event formatting).
//
// SnapshotProbe embeds LogProbeCommon to inherit the shared configuration
// fields and implements the ir.ProbeDefinition interface via the methods
// defined below.
type SnapshotProbe struct {
	LogProbeCommon
	// CaptureSnapshot is always true for snapshot probes.
	CaptureSnapshot True `json:"captureSnapshot"`
}

func (l *SnapshotProbe) validate() error {
	return validateWhere(l.Where)
}

// GetKind returns the kind of the probe.
func (l *SnapshotProbe) GetKind() ir.ProbeKind {
	return ir.ProbeKindSnapshot
}

// GetThrottleConfig returns the throttle configuration of the probe.
func (l *SnapshotProbe) GetThrottleConfig() ir.ThrottleConfig {
	return (*snapshotThrottleConfig)(l.Sampling)
}

// SpanProbe is a probe that decorates a span.
type SpanProbe struct {
	ProbeCommon
}

func (s *SpanProbe) validate() error {
	if err := validateWhere(s.Where); err != nil {
		return err
	}
	return nil
}

// GetCaptureConfig returns the capture configuration of the probe.
func (s *SpanProbe) GetCaptureConfig() ir.CaptureConfig { return noCaptureConfig{} }

// GetKind returns the kind of the probe.
func (s *SpanProbe) GetKind() ir.ProbeKind { return ir.ProbeKindSpan }

// GetThrottleConfig returns the throttle configuration of the probe.
func (s *SpanProbe) GetThrottleConfig() ir.ThrottleConfig { return infiniteThrottleConfig{} }

// Exists so that we can make accessors infallible. In practice, valid
// probes won't return this.
type noWhere struct{}

var _ ir.Where = noWhere{}

func (noWhere) Where() {}

type functionWhere Where

var _ ir.FunctionWhere = (*functionWhere)(nil)

func (w *functionWhere) Location() string {
	return w.MethodName
}

func (w *functionWhere) Where() {}

func getWhere(where *Where) ir.Where {
	if where == nil {
		return noWhere{}
	}
	if where.MethodName != "" {
		return (*functionWhere)(where)
	}
	// TODO: support other where types like lines.
	return noWhere{}
}

type irCaptureConfig Capture

var _ ir.CaptureConfig = (*irCaptureConfig)(nil)

func (c *irCaptureConfig) GetMaxReferenceDepth() uint32 {
	if c == nil || c.MaxReferenceDepth == 0 {
		return math.MaxUint32
	}
	return uint32(c.MaxReferenceDepth)
}
func (c *irCaptureConfig) GetMaxFieldCount() uint32 {
	if c == nil || c.MaxFieldCount == 0 {
		return math.MaxUint32
	}
	return uint32(c.MaxFieldCount)
}
func (c *irCaptureConfig) GetMaxCollectionSize() uint32 {
	if c == nil || c.MaxCollectionSize == 0 {
		return math.MaxUint32
	}
	return uint32(c.MaxCollectionSize)
}

type noCaptureConfig struct{}

var _ ir.CaptureConfig = noCaptureConfig{}

func (noCaptureConfig) GetMaxReferenceDepth() uint32 { return 0 }
func (noCaptureConfig) GetMaxFieldCount() uint32     { return 0 }
func (noCaptureConfig) GetMaxCollectionSize() uint32 { return 0 }

type logThrottleConfig Sampling

// logThrottleConfig is a throttle configuration for log probes that is
var _ ir.ThrottleConfig = (*logThrottleConfig)(nil)

func (c *logThrottleConfig) GetThrottlePeriodMs() uint32 { return 100 }
func (c *logThrottleConfig) GetThrottleBudget() int64    { return 500 }

type snapshotThrottleConfig Sampling

var _ ir.ThrottleConfig = (*snapshotThrottleConfig)(nil)

func (c *snapshotThrottleConfig) GetThrottlePeriodMs() uint32 { return 1000 }
func (c *snapshotThrottleConfig) GetThrottleBudget() int64 {
	if c == nil || c.SnapshotsPerSecond <= 0 {
		return 1
	}
	return int64(c.SnapshotsPerSecond)
}

type infiniteThrottleConfig struct{}

var _ ir.ThrottleConfig = infiniteThrottleConfig{}

func (infiniteThrottleConfig) GetThrottlePeriodMs() uint32 { return 1000 }
func (infiniteThrottleConfig) GetThrottleBudget() int64    { return math.MaxInt64 }

func validateWhere(where *Where) error {
	if where == nil {
		return errors.New("where is required")
	}
	if where.SourceFile != "" && len(where.Lines) > 0 {
		return errors.New("sourceFile and lines are not supported")
	}
	if where.Signature != "" {
		return errors.New("signature is not supported")
	}
	if where.MethodName == "" {
		return errors.New("methodName must be set for probes")
	}
	return nil
}

// True represents a value that marshals to json `true`.
type True struct{}

// MarshalJSON implements json.Marshaler.
func (True) MarshalJSON() ([]byte, error) {
	return trueJSON, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (True) UnmarshalJSON(data []byte) error {
	if !bytes.Equal(data, trueJSON) {
		return fmt.Errorf("expected true, got %s", data)
	}
	return nil
}

var trueJSON = []byte("true")

// False represents a value that marshals to json `false`.
type False struct{}

// MarshalJSON implements json.Marshaler.
func (False) MarshalJSON() ([]byte, error) {
	return falseJSON, nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (False) UnmarshalJSON(data []byte) error {
	if !bytes.Equal(data, falseJSON) {
		return fmt.Errorf("expected false, got %s", data)
	}
	return nil
}

var falseJSON = []byte("false")
