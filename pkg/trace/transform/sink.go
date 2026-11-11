// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package transform

import (
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

// spanSink is the destination of an OTel span conversion. The conversion logic in this package
// is written once against spanSink and shared by the pb (OtelSpanToDDSpan) and idx
// (OtelSpanToIdxSpan) outputs.
//
// The sink exposes the pb span model: Meta (string tags) and Metrics (numeric tags) are separate
// namespaces, and a key can be present in both. Implementations must preserve that model for
// every read made during the conversion, since the conversion decides what to write from what is
// already there (set-if-absent, set-if-empty, and so on).
type spanSink interface {
	service() string
	setService(string)
	name() string
	setName(string)
	resource() string
	setResource(string)
	spanType() string
	setSpanType(string)
	setError(int32)

	// meta returns the Meta value of k and whether k is present in Meta.
	meta(k string) (string, bool)
	setMeta(k, v string)
	// metric returns the Metrics value of k and whether k is present in Metrics.
	metric(k string) (float64, bool)
	setMetric(k string, v float64)

	setEvents(ptrace.SpanEventSlice)
	setLinks(ptrace.SpanLinkSlice)
}

// metaSink is the part of spanSink that only reads and writes Meta.
type metaSink interface {
	meta(k string) (string, bool)
	setMeta(k, v string)
}

// metaAccessor adapts the Meta of a metaSink to a semantics.Accessor, with the same behavior as
// semantics.StringMapAccessor over the pb span Meta map.
type metaAccessor struct{ s metaSink }

func (a metaAccessor) GetString(key string) string {
	v, _ := a.s.meta(key)
	return v
}

func (a metaAccessor) GetInt64(string) (int64, bool) { return 0, false }

func (a metaAccessor) GetFloat64(string) (float64, bool) { return 0, false }

// pbSpanSink writes the conversion directly into a pb.Span. Its methods are the plain map and
// field operations the conversion used before spanSink existed, so the pb output is unchanged.
type pbSpanSink pb.Span

var _ spanSink = (*pbSpanSink)(nil)

func (s *pbSpanSink) service() string      { return s.Service }
func (s *pbSpanSink) setService(v string)  { s.Service = v }
func (s *pbSpanSink) name() string         { return s.Name }
func (s *pbSpanSink) setName(v string)     { s.Name = v }
func (s *pbSpanSink) resource() string     { return s.Resource }
func (s *pbSpanSink) setResource(v string) { s.Resource = v }
func (s *pbSpanSink) spanType() string     { return s.Type }
func (s *pbSpanSink) setSpanType(v string) { s.Type = v }
func (s *pbSpanSink) setError(v int32)     { s.Error = v }

func (s *pbSpanSink) meta(k string) (string, bool) {
	v, ok := s.Meta[k]
	return v, ok
}

func (s *pbSpanSink) setMeta(k, v string) {
	if s.Meta == nil {
		s.Meta = make(map[string]string)
	}
	s.Meta[k] = v
}

func (s *pbSpanSink) metric(k string) (float64, bool) {
	v, ok := s.Metrics[k]
	return v, ok
}

func (s *pbSpanSink) setMetric(k string, v float64) {
	if s.Metrics == nil {
		s.Metrics = make(map[string]float64)
	}
	s.Metrics[k] = v
}

func (s *pbSpanSink) setEvents(events ptrace.SpanEventSlice) {
	s.SpanEvents = OTelSpanEventsToDD(events)
}

func (s *pbSpanSink) setLinks(links ptrace.SpanLinkSlice) {
	s.SpanLinks = OTelSpanLinksToDD(links)
}

// mapMetaSink is a metaSink over a plain Meta map, used by the exported helpers that take one.
type mapMetaSink map[string]string

func (m mapMetaSink) meta(k string) (string, bool) {
	v, ok := m[k]
	return v, ok
}

func (m mapMetaSink) setMeta(k, v string) { m[k] = v }
