// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcjson

import (
	"encoding/json"
	"fmt"
)

// UnmarshalProbe unmarshals a Probe from a JSON byte slice.
func UnmarshalProbe(data []byte) (Probe, error) {
	type config struct {
		ID   string `json:"id"`
		Type Type   `json:"type"`
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
		return nil, fmt.Errorf("UnmarshalProbe: unexpected config.Type: %#v", c.Type)
	case TypeLogProbe:
		v = new(LogProbe)
	case TypeMetricProbe:
		v = new(MetricProbe)
	case TypeSpanProbe:
		v = new(SpanProbe)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return nil, fmt.Errorf(
			"UnmarshalProbe: id: %s, type: %s: failed to parse json: %w",
			c.ID, c.Type, err,
		)
	}
	return v, nil
}

// Probe is the interface for all probe types.
type Probe interface {
	// GetID returns the ID of the probe.
	GetID() string
	// GetType returns the type of the probe.
	GetType() Type
	// GetVersion returns the version of the probe.
	GetVersion() int
	// GetWhere returns the where clause for the probe.
	GetWhere() *Where
	// GetTags returns the tags for the probe.
	GetTags() []string
}

// These are the data structures for the external interface

// Where for the remote config data.
type Where struct {
	TypeName   string   `json:"typeName,omitempty"`
	MethodName string   `json:"methodName,omitempty"`
	SourceFile string   `json:"sourceFile,omitempty"`
	Signature  string   `json:"signature,omitempty"`
	Lines      []string `json:"lines,omitempty"`
}

// When for the remote config data.
type When struct {
	DSL  string          `json:"dsl"`
	JSON json.RawMessage `json:"json"`
}

// Value for the remote config data.
type Value struct {
	DSL  string          `json:"dsl,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
}

// Capture for the remote config data.
type Capture struct {
	MaxReferenceDepth int `json:"maxReferenceDepth"`
	MaxFieldCount     int `json:"maxFieldCount,omitempty"`
	MaxCollectionSize int `json:"maxCollectionSize,omitempty"`
}

// Sampling for the remote config data.
type Sampling struct {
	SnapshotsPerSecond float64 `json:"snapshotsPerSecond"`
}

// MetricProbe for the remote config data.
type MetricProbe struct {
	ID         string   `json:"id"`
	Version    int      `json:"version"`
	Type       string   `json:"type"`
	Language   string   `json:"language"`
	Where      *Where   `json:"where"`
	Tags       []string `json:"tags"`
	Kind       string   `json:"kind"`
	MetricName string   `json:"metricName"`
	Value      *Value   `json:"value,omitempty"`
	EvaluateAt string   `json:"evaluateAt,omitempty"`
}

// GetID implements Probe.
func (m *MetricProbe) GetID() string {
	return m.ID
}

// GetType implements Probe.
func (m *MetricProbe) GetType() Type {
	return TypeMetricProbe
}

// GetWhere implements Probe.
func (m *MetricProbe) GetWhere() *Where {
	return m.Where
}

// GetVersion implements Probe.
func (m *MetricProbe) GetVersion() int {
	return m.Version
}

// GetTags implements Probe.
func (m *MetricProbe) GetTags() []string {
	return m.Tags
}

// LogProbe for the remote config data.
type LogProbe struct {
	ID              string            `json:"id"`
	Version         int               `json:"version"`
	Type            string            `json:"type"`
	Language        string            `json:"language"`
	Where           *Where            `json:"where"`
	When            *When             `json:"when,omitempty"`
	Tags            []string          `json:"tags"`
	Template        string            `json:"template"`
	Segments        []json.RawMessage `json:"segments"`
	CaptureSnapshot bool              `json:"captureSnapshot"`
	Capture         *Capture          `json:"capture,omitempty"`
	Sampling        *Sampling         `json:"sampling,omitempty"`
	EvaluateAt      string            `json:"evaluateAt,omitempty"`
}

// GetID implements Probe.
func (l *LogProbe) GetID() string {
	return l.ID
}

// GetType implements Probe.
func (l *LogProbe) GetType() Type {
	return TypeLogProbe
}

// GetWhere implements Probe.
func (l *LogProbe) GetWhere() *Where {
	return l.Where
}

// GetVersion implements Probe.
func (l *LogProbe) GetVersion() int {
	return l.Version
}

// GetTags implements Probe.
func (l *LogProbe) GetTags() []string {
	return l.Tags
}

// SpanProbe for the remote config data.
type SpanProbe struct {
	ID         string   `json:"id"`
	Version    int      `json:"version"`
	Type       string   `json:"type"`
	Language   string   `json:"language"`
	Where      *Where   `json:"where"`
	Tags       []string `json:"tags"`
	EvaluateAt string   `json:"evaluateAt,omitempty"`
}

// GetID implements Probe.
func (l *SpanProbe) GetID() string {
	return l.ID
}

// GetType implements Probe.
func (l *SpanProbe) GetType() Type {
	return TypeSpanProbe
}

// GetWhere implements Probe.
func (l *SpanProbe) GetWhere() *Where {
	return l.Where
}

// GetVersion implements Probe.
func (l *SpanProbe) GetVersion() int {
	return l.Version
}

// GetTags implements Probe.
func (l *SpanProbe) GetTags() []string {
	return l.Tags
}
