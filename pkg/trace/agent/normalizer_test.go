// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func newTestSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   rand.Uint64(),
		Start:    1448466874000000000,
		TraceID:  424242,
		Meta: map[string]string{
			"user": "leo",
			"pool": "fondue",
		},
		Metrics: map[string]float64{
			"cheese_weight": 100000.0,
		},
		ParentID: 1111,
		Type:     "http",
	}
}

func newTagStats() *info.TagStats {
	return &info.TagStats{Stats: info.Stats{TracesDropped: &info.TracesDropped{}, SpansMalformed: &info.SpansMalformed{}}}
}

// tsMalformed returns a new info.TagStats structure containing the given malformed stats.
func tsMalformed(tm *info.SpansMalformed) *info.TagStats {
	return &info.TagStats{Stats: info.Stats{SpansMalformed: tm, TracesDropped: &info.TracesDropped{}}}
}

// tagStatsDropped returns a new info.TagStats structure containing the given dropped stats.
func tsDropped(td *info.TracesDropped) *info.TagStats {
	return &info.TagStats{Stats: info.Stats{SpansMalformed: &info.SpansMalformed{}, TracesDropped: td}}
}

func TestNormalizeOK(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeServicePassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Service
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Service)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyServiceNoLang(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Service = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, traceutil.DefaultServiceName, s.Service)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ServiceEmpty: 1}), ts)
}

func TestNormalizeEmptyServiceWithLang(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Service = ""
	ts.Lang = "java"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Service, fmt.Sprintf("unnamed-%s-service", ts.Lang))
	tsExpected := tsMalformed(&info.SpansMalformed{ServiceEmpty: 1})
	tsExpected.Lang = ts.Lang
	assert.Equal(t, tsExpected, ts)
}

func TestNormalizeLongService(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Service, s.Service[:traceutil.MaxServiceLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ServiceTruncate: 1}), ts)
}

func TestNormalizeNamePassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Name
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Name)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyName(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Name = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, traceutil.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameEmpty: 1}), ts)
}

func TestNormalizeLongName(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, s.Name[:traceutil.MaxNameLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameTruncate: 1}), ts)
}

func TestNormalizeNameNoAlphanumeric(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Name = "/"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, traceutil.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameInvalid: 1}), ts)
}

func TestNormalizeNameForMetrics(t *testing.T) {
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	ts := newTagStats()
	s := newTestSpan()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, normalize(ts, s))
		assert.Equal(t, expName, s.Name)
		assert.Equal(t, newTagStats(), ts)
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Resource
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Resource)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyResource(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Resource = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Resource, s.Name)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ResourceEmpty: 1}), ts)
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.TraceID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.TraceID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNoTraceID(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.TraceID = 0
	assert.Error(t, normalize(ts, s))
	assert.Equal(t, tsDropped(&info.TracesDropped{TraceIDZero: 1}), ts)
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.SpanID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.SpanID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNoSpanID(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.SpanID = 0
	assert.Error(t, normalize(ts, s))
	assert.Equal(t, tsDropped(&info.TracesDropped{SpanIDZero: 1}), ts)
}

func TestNormalizeStart(t *testing.T) {
	t.Run("pass-through", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		before := s.Start
		assert.NoError(t, normalize(ts, s))
		assert.Equal(t, before, s.Start)
		assert.Equal(t, newTagStats(), ts)
	})

	t.Run("too-small", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		s.Start = 42
		minStart := time.Now().UnixNano() - s.Duration
		assert.NoError(t, normalize(ts, s))
		assert.True(t, s.Start >= minStart)
		assert.True(t, s.Start <= time.Now().UnixNano()-s.Duration)
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: 1}), ts)
	})

	t.Run("too-small-with-large-duration", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		s.Start = 42
		s.Duration = time.Now().UnixNano() * 2
		minStart := time.Now().UnixNano()
		assert.NoError(t, normalize(ts, s))
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: 1}), ts)
		assert.True(t, s.Start >= minStart, "start should have been reset to current time")
		assert.True(t, s.Start <= time.Now().UnixNano(), "start should have been reset to current time")
	})
}

func TestNormalizeDurationPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Duration
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Duration)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = 0
	assert.NoError(t, normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNegativeDuration(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = -50
	assert.NoError(t, normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidDuration: 1}), ts)
}

func TestNormalizeLargeDuration(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = int64(math.MaxInt64)
	assert.NoError(t, normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidDuration: 1}), ts)
}

func TestNormalizeErrorPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Error
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Error)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Metrics
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Metrics)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Meta
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Meta)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.ParentID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.ParentID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTypePassThru(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	before := s.Type
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Type)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Type = strings.Repeat("sql", 1000)
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, tsMalformed(&info.SpansMalformed{TypeTruncate: 1}), ts)
}

func TestNormalizeServiceTag(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Service = "retargeting(api-Staging "
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, "retargeting_api-staging", s.Service)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEnv(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.Meta["env"] = "DEVELOPMENT"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, "development", s.Meta["env"])
	assert.Equal(t, newTagStats(), ts)
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	ts := newTagStats()
	s := newTestSpan()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, uint64(0), s.ParentID)
	assert.Equal(t, beforeTraceID, s.TraceID)
	assert.Equal(t, beforeSpanID, s.SpanID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTraceEmpty(t *testing.T) {
	ts, trace := newTagStats(), pb.Trace{}
	err := normalizeTrace(ts, trace)
	assert.Error(t, err)
	assert.Equal(t, tsDropped(&info.TracesDropped{EmptyTrace: 1}), ts)
}

func TestNormalizeTraceTraceIdMismatch(t *testing.T) {
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span1.TraceID = 1
	span2.TraceID = 2
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.Error(t, err)
	assert.Equal(t, tsDropped(&info.TracesDropped{ForeignSpan: 1}), ts)
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.Name = "" // invalid
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameEmpty: 1}), ts)
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.SpanID = span1.SpanID
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{DuplicateSpanID: 1}), ts)
}

func TestNormalizeTrace(t *testing.T) {
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.SpanID++
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.NoError(t, err)
}

func TestIsValidStatusCode(t *testing.T) {
	assert := assert.New(t)
	assert.True(isValidStatusCode("100"))
	assert.True(isValidStatusCode("599"))
	assert.False(isValidStatusCode("99"))
	assert.False(isValidStatusCode("600"))
	assert.False(isValidStatusCode("Invalid status code"))
}

func BenchmarkNormalization(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ts := newTagStats()
		span := newTestSpan()
		ts.Lang = "go"

		normalize(ts, span)
	}
}
