// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
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
		SpanLinks: []*pb.SpanLink{
			{TraceID: uint64(3), TraceIDHigh: uint64(2), SpanID: uint64(1), Attributes: map[string]string{"link.name": "name"}, Tracestate: "", Flags: uint32(0)},
		},
	}
}

// GetTestSpan returns a Span with different fields set
func newTestSpanV1(strings *idx.StringTable) *idx.InternalSpan {
	return idx.NewInternalSpan(strings, &idx.Span{
		SpanID:      rand.Uint64(),
		ParentID:    1111,
		ServiceRef:  strings.Add("django"),
		NameRef:     strings.Add("django.controller"),
		ResourceRef: strings.Add("GET /some/raclette"),
		Start:       1448466874000000000,
		Duration:    10000000,
		Attributes: map[uint32]*idx.AnyValue{
			strings.Add("user"): {
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: strings.Add("leo"),
				},
			},
			strings.Add("pool"): {
				Value: &idx.AnyValue_StringValueRef{
					StringValueRef: strings.Add("fondue"),
				},
			},
			strings.Add("cheese_weight"): {
				Value: &idx.AnyValue_DoubleValue{
					DoubleValue: 100000.0,
				},
			},
		},
		Links: []*idx.SpanLink{
			{
				TraceID: []byte{42},
				SpanID:  1,
				Attributes: map[uint32]*idx.AnyValue{
					strings.Add("link.name"): {
						Value: &idx.AnyValue_StringValueRef{
							StringValueRef: strings.Add("name"),
						},
					},
				},
				TracestateRef: 0,
				Flags:         0,
			},
		},
	})
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
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeServicePassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Meta[peerServiceKey] = "foo"
	s.Meta[baseServiceKey] = "bar"
	before := s.Service
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Service)
	assert.Equal(t, "foo", s.Meta[peerServiceKey])
	assert.Equal(t, "bar", s.Meta[baseServiceKey])
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyServiceNoLang(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Service = ""
	s.Meta[peerServiceKey] = ""
	s.Meta[baseServiceKey] = ""
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, normalize.DefaultServiceName, s.Service)
	assert.Equal(t, "", s.Meta[peerServiceKey]) // no fallback on peer service tag
	assert.Equal(t, "", s.Meta[baseServiceKey]) // no fallback on base service tag
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ServiceEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeEmptyServiceWithLang(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Service = ""
	ts.Lang = "java"
	s.Meta[peerServiceKey] = ""
	s.Meta[baseServiceKey] = ""
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Service, fmt.Sprintf("unnamed-%s-service", ts.Lang))
	assert.Equal(t, "", s.Meta[peerServiceKey]) // no fallback on peer service tag
	assert.Equal(t, "", s.Meta[baseServiceKey]) // no fallback on base service tag
	tsExpected := tsMalformed(&info.SpansMalformed{ServiceEmpty: *atomic.NewInt64(1)})
	tsExpected.Lang = ts.Lang
	assert.Equal(t, tsExpected, ts)
}

func TestNormalizeLongService(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	s.Meta[peerServiceKey] = strings.Repeat("BRIE", 100)
	s.Meta[baseServiceKey] = strings.Repeat("ROQUEFORT", 100)
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Service, s.Service[:normalize.MaxServiceLen])
	assert.Equal(t, s.Meta[peerServiceKey], s.Meta[peerServiceKey][:normalize.MaxServiceLen])
	assert.Equal(t, s.Meta[baseServiceKey], s.Meta[baseServiceKey][:normalize.MaxServiceLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{
		ServiceTruncate:     *atomic.NewInt64(1),
		PeerServiceTruncate: *atomic.NewInt64(1),
		BaseServiceTruncate: *atomic.NewInt64(1),
	}), ts)
}

func TestNormalizeNamePassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Name
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Name)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyName(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Name = ""
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Name, normalize.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeSpanLinkName(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()

	// Normalize a span that contains an empty link name
	emptyLinkNameSpan := newTestSpan()
	emptyLinkNameSpan.SpanLinks[0].Attributes["link.name"] = ""
	assert.NoError(t, a.normalize(ts, emptyLinkNameSpan))
	assert.Equal(t, emptyLinkNameSpan.SpanLinks[0].Attributes["link.name"], normalize.DefaultSpanName)

	// Normalize a span that contains an invalid link name
	invalidLinkNameSpan := newTestSpan()
	invalidLinkNameSpan.SpanLinks[0].Attributes["link.name"] = "!@#$%^&*()_+"
	assert.NoError(t, a.normalize(ts, invalidLinkNameSpan))
	assert.Equal(t, invalidLinkNameSpan.SpanLinks[0].Attributes["link.name"], normalize.DefaultSpanName)

	// Normalize a span that contains a valid link name
	validLinkNameSpan := newTestSpan()
	validLinkNameSpan.SpanLinks[0].Attributes["link.name"] = "valid_name"
	assert.NoError(t, a.normalize(ts, validLinkNameSpan))
	assert.Equal(t, validLinkNameSpan.SpanLinks[0].Attributes["link.name"], "valid_name")
}

func TestNormalizeLongName(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Name, s.Name[:normalize.MaxNameLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameTruncate: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeNameNoAlphanumeric(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Name = "/"
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Name, normalize.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameInvalid: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeNameForMetrics(t *testing.T) {
	a := &Agent{conf: config.New()}
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	ts := newTagStats()
	s := newTestSpan()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, a.normalize(ts, s))
		assert.Equal(t, expName, s.Name)
		assert.Equal(t, newTagStats(), ts)
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Resource
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Resource)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyResource(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Resource = ""
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, s.Resource, s.Name)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ResourceEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.TraceID
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.TraceID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNoTraceID(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.TraceID = 0
	assert.Error(t, a.normalize(ts, s))
	assert.Equal(t, tsDropped(&info.TracesDropped{TraceIDZero: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeComponent2Name(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	assert := assert.New(t)

	t.Run("on", func(t *testing.T) {
		a := &Agent{conf: config.New()}
		a.conf.Features = map[string]struct{}{"component2name": {}}

		t.Run("with", func(_ *testing.T) {
			s := newTestSpan()
			s.Meta["component"] = "component"
			assert.NoError(a.normalize(ts, s))
			assert.Equal(s.Name, "component")
		})

		t.Run("without", func(_ *testing.T) {
			s := newTestSpan()
			assert.Empty(s.Meta["component"])
			assert.NoError(a.normalize(ts, s))
			assert.Equal(s.Name, "django.controller")
		})
	})

	t.Run("off", func(_ *testing.T) {
		s := newTestSpan()
		s.Meta["component"] = "component"
		assert.NoError(a.normalize(ts, s))
		assert.Equal(s.Name, "django.controller")
	})
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.SpanID
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.SpanID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNoSpanID(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.SpanID = 0
	assert.Error(t, a.normalize(ts, s))
	assert.Equal(t, tsDropped(&info.TracesDropped{SpanIDZero: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeStart(t *testing.T) {
	a := &Agent{conf: config.New()}
	t.Run("pass-through", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		before := s.Start
		assert.NoError(t, a.normalize(ts, s))
		assert.Equal(t, before, s.Start)
		assert.Equal(t, newTagStats(), ts)
	})

	t.Run("too-small", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		s.Start = 42
		minStart := time.Now().UnixNano() - s.Duration
		assert.NoError(t, a.normalize(ts, s))
		assert.True(t, s.Start >= minStart)
		assert.True(t, s.Start <= time.Now().UnixNano()-s.Duration)
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: *atomic.NewInt64(1)}), ts)
	})

	t.Run("too-small-with-large-duration", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpan()
		s.Start = 42
		s.Duration = time.Now().UnixNano() * 2
		minStart := time.Now().UnixNano()
		assert.NoError(t, a.normalize(ts, s))
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: *atomic.NewInt64(1)}), ts)
		assert.True(t, s.Start >= minStart, "start should have been reset to current time")
		assert.True(t, s.Start <= time.Now().UnixNano(), "start should have been reset to current time")
	})
}

func TestNormalizeDurationPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Duration
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Duration)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = 0
	assert.NoError(t, a.normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeNegativeDuration(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = -50
	assert.NoError(t, a.normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidDuration: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeLargeDuration(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Duration = int64(math.MaxInt64)
	assert.NoError(t, a.normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidDuration: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeErrorPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Error
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Error)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Metrics
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Metrics)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Meta
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Meta)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.ParentID
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.ParentID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTypePassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	before := s.Type
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, before, s.Type)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Type = strings.Repeat("sql", 1000)
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, tsMalformed(&info.SpansMalformed{TypeTruncate: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeServiceTag(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Service = "retargeting(api-Staging "
	s.Meta[peerServiceKey] = "retargeting(api-Peer "
	s.Meta[baseServiceKey] = "retargeting(api-Base "
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, "retargeting_api-staging", s.Service)
	assert.Equal(t, "retargeting_api-peer", s.Meta[peerServiceKey])
	assert.Equal(t, "retargeting_api-base", s.Meta[baseServiceKey])
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEnv(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.Meta["env"] = "123DEVELOPMENT"
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, "123development", s.Meta["env"])
	assert.Equal(t, newTagStats(), ts)
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpan()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	assert.NoError(t, a.normalize(ts, s))
	assert.Equal(t, uint64(0), s.ParentID)
	assert.Equal(t, beforeTraceID, s.TraceID)
	assert.Equal(t, beforeSpanID, s.SpanID)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTraceEmpty(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts, trace := newTagStats(), pb.Trace{}
	err := a.normalizeTrace(ts, trace)
	assert.Error(t, err)
	assert.Equal(t, tsDropped(&info.TracesDropped{EmptyTrace: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTraceTraceIdMismatch(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span1.TraceID = 1
	span2.TraceID = 2
	trace := pb.Trace{span1, span2}
	err := a.normalizeTrace(ts, trace)
	assert.Error(t, err)
	assert.Equal(t, tsDropped(&info.TracesDropped{ForeignSpan: *atomic.NewInt64(1)}), ts)
}

// TestNormalizeTraceTraceIdMismatch128Bit tests that spans with matching low 64 bits
// but different high 64 bits (_dd.p.tid) are correctly detected as foreign spans.
//
// Note: In normal operation, two spans in the same chunk would NOT both have _dd.p.tid.
// Tracers only set _dd.p.tid on the first span in each chunk to avoid redundant data.
// This test validates an edge case where this invariant is violated.
func TestNormalizeTraceTraceIdMismatch128Bit(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	// Same low 64 bits, different high 64 bits (_dd.p.tid)
	span1.TraceID = 1
	span2.TraceID = 1
	span1.Meta["_dd.p.tid"] = "0000000000000001"
	span2.Meta["_dd.p.tid"] = "0000000000000002"

	trace := pb.Trace{span1, span2}
	err := a.normalizeTrace(ts, trace)
	assert.Error(t, err)
	assert.Equal(t, tsDropped(&info.TracesDropped{ForeignSpan: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.Name = "" // invalid
	trace := pb.Trace{span1, span2}
	err := a.normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.SpanID = span1.SpanID
	trace := pb.Trace{span1, span2}
	err := a.normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{DuplicateSpanID: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTrace(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	span1, span2 := newTestSpan(), newTestSpan()

	span2.SpanID++
	trace := pb.Trace{span1, span2}
	err := a.normalizeTrace(ts, trace)
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

func TestNormalizeChunkPopulatingOrigin(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	traceutil.SetMeta(root, "_dd.origin", "rum")
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Origin = ""
	setChunkAttributes(chunk, root)
	assert.Equal("rum", chunk.Origin)
}

func TestNormalizeChunkNotPopulatingOrigin(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	traceutil.SetMeta(root, "_dd.origin", "rum")
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Origin = "cloudrun"
	setChunkAttributes(chunk, root)
	assert.Equal("cloudrun", chunk.Origin)
}

func TestNormalizeChunkPopulatingSamplingPriority(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	traceutil.SetMetric(root, "_sampling_priority_v1", float64(sampler.PriorityAutoKeep))
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Priority = int32(sampler.PriorityNone)
	setChunkAttributes(chunk, root)
	assert.EqualValues(sampler.PriorityAutoKeep, chunk.Priority)
}

func TestNormalizeChunkNotPopulatingSamplingPriority(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	traceutil.SetMetric(root, "_sampling_priority_v1", float64(sampler.PriorityAutoKeep))
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Priority = int32(sampler.PriorityAutoDrop)
	setChunkAttributes(chunk, root)
	assert.EqualValues(sampler.PriorityAutoDrop, chunk.Priority)
}

func TestNormalizePopulatePriorityFromAnySpan(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Priority = int32(sampler.PriorityNone)
	chunk.Spans = []*pb.Span{newTestSpan(), newTestSpan(), newTestSpan()}
	chunk.Spans[0].Metrics = nil
	chunk.Spans[2].Metrics = nil
	traceutil.SetMetric(chunk.Spans[1], "_sampling_priority_v1", float64(sampler.PriorityAutoKeep))
	setChunkAttributes(chunk, root)
	assert.EqualValues(sampler.PriorityAutoKeep, chunk.Priority)
}

func TestTagDecisionMaker(t *testing.T) {
	assert := assert.New(t)
	root := newTestSpan()
	chunk := testutil.TraceChunkWithSpan(root)
	chunk.Priority = int32(sampler.PriorityNone)
	chunk.Spans = []*pb.Span{newTestSpan(), newTestSpan(), newTestSpan()}
	chunk.Spans[0].Metrics = nil
	chunk.Spans[2].Metrics = nil
	traceutil.SetMeta(chunk.Spans[1], tagDecisionMaker, "right")
	traceutil.SetMeta(chunk.Spans[2], tagDecisionMaker, "wrong")
	setChunkAttributes(chunk, root)
	assert.Equal("right", chunk.Tags[tagDecisionMaker])
	assert.Equal("right", chunk.Spans[1].Meta[tagDecisionMaker])
}

func BenchmarkNormalization(b *testing.B) {
	a := &Agent{conf: config.New()}
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ts := newTagStats()
		span := newTestSpan()
		ts.Lang = "go"

		a.normalize(ts, span)
	}
}

func TestLexerNormalization(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.SQLObfuscationMode = string(obfuscate.ObfuscateAndNormalize)
	agnt := NewAgent(ctx, cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, gzip.NewComponent())
	defer cancelFunc()
	span := &pb.Span{
		Resource: "SELECT * FROM [u].[users]",
		Type:     "sql",
		Meta:     map[string]string{"db.type": "sqlserver"},
	}
	agnt.obfuscateSpan(span)
	assert.Equal(t, "SELECT * FROM u.users", span.Resource)
}

func TestNormalizeServicePassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetService("foo")
	s.SetStringAttribute(peerServiceKey, "foo")
	s.SetStringAttribute(baseServiceKey, "bar")
	before := s.Service()
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, before, s.Service())
	peerSvc, _ := s.GetAttributeAsString(peerServiceKey)
	baseSvc, _ := s.GetAttributeAsString(baseServiceKey)
	assert.Equal(t, "foo", peerSvc)
	assert.Equal(t, "bar", baseSvc)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyServiceNoLangV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetService("")
	s.SetStringAttribute(peerServiceKey, "")
	s.SetStringAttribute(baseServiceKey, "")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, normalize.DefaultServiceName, s.Service())
	peerSvc, _ := s.GetAttributeAsString(peerServiceKey)
	baseSvc, _ := s.GetAttributeAsString(baseServiceKey)
	assert.Equal(t, "", peerSvc) // no fallback on peer service tag
	assert.Equal(t, "", baseSvc) // no fallback on base service tag
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ServiceEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeEmptyServiceWithLangV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetService("")
	ts.Lang = "java"
	s.SetStringAttribute(peerServiceKey, "")
	s.SetStringAttribute(baseServiceKey, "")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Service(), fmt.Sprintf("unnamed-%s-service", ts.Lang))
	peerSvc, _ := s.GetAttributeAsString(peerServiceKey)
	baseSvc, _ := s.GetAttributeAsString(baseServiceKey)
	assert.Equal(t, "", peerSvc) // no fallback on peer service tag
	assert.Equal(t, "", baseSvc) // no fallback on base service tag
	tsExpected := tsMalformed(&info.SpansMalformed{ServiceEmpty: *atomic.NewInt64(1)})
	tsExpected.Lang = ts.Lang
	assert.Equal(t, tsExpected, ts)
}

func TestNormalizeLongServiceV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetService(strings.Repeat("CAMEMBERT", 100))
	s.SetStringAttribute(peerServiceKey, strings.Repeat("BRIE", 100))
	s.SetStringAttribute(baseServiceKey, strings.Repeat("ROQUEFORT", 100))
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Service(), s.Service()[:normalize.MaxServiceLen])
	peerSvc, _ := s.GetAttributeAsString(peerServiceKey)
	baseSvc, _ := s.GetAttributeAsString(baseServiceKey)
	assert.Equal(t, peerSvc, peerSvc[:normalize.MaxServiceLen])
	assert.Equal(t, baseSvc, baseSvc[:normalize.MaxServiceLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{
		ServiceTruncate:     *atomic.NewInt64(1),
		PeerServiceTruncate: *atomic.NewInt64(1),
		BaseServiceTruncate: *atomic.NewInt64(1),
	}), ts)
}

func TestNormalizeNamePassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	before := s.Name()
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, before, s.Name())
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyNameV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetName("")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Name(), normalize.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeLongNameV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetName(strings.Repeat("CAMEMBERT", 100))
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Name(), s.Name()[:normalize.MaxNameLen])
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameTruncate: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeNameNoAlphanumericV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetName("/")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Name(), normalize.DefaultSpanName)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{SpanNameInvalid: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeResourcePassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	before := s.Resource()
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, before, s.Resource())
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyResourceV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetResource("")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, s.Resource(), s.Name())
	assert.Equal(t, tsMalformed(&info.SpansMalformed{ResourceEmpty: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeNoSpanIDV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetSpanID(0)
	assert.Error(t, a.normalizeV1(ts, s))
	assert.Equal(t, tsDropped(&info.TracesDropped{SpanIDZero: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeStartV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	t.Run("pass-through", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpanV1(idx.NewStringTable())
		before := s.Start()
		assert.NoError(t, a.normalizeV1(ts, s))
		assert.Equal(t, before, s.Start())
		assert.Equal(t, newTagStats(), ts)
	})

	t.Run("too-small", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpanV1(idx.NewStringTable())
		s.SetStart(42)
		minStart := time.Now().UnixNano() - int64(s.Duration())
		assert.NoError(t, a.normalizeV1(ts, s))
		assert.True(t, s.Start() >= uint64(minStart))
		assert.True(t, s.Start() <= uint64(time.Now().UnixNano())-s.Duration())
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: *atomic.NewInt64(1)}), ts)
	})

	t.Run("too-small-with-large-duration", func(t *testing.T) {
		ts := newTagStats()
		s := newTestSpanV1(idx.NewStringTable())
		s.SetStart(42)
		s.SetDuration(uint64(time.Now().UnixNano() * 2))
		minStart := time.Now().UnixNano()
		assert.NoError(t, a.normalizeV1(ts, s))
		assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidStartDate: *atomic.NewInt64(1)}), ts)
		assert.True(t, s.Start() >= uint64(minStart), "start should have been reset to current time")
		assert.True(t, s.Start() <= uint64(time.Now().UnixNano()), "start should have been reset to current time")
	})
}

func TestNormalizeDurationPassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	before := s.Duration()
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, before, s.Duration())
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEmptyDurationV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetDuration(0)
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.EqualValues(t, s.Duration(), 0)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeLargeDurationV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetDuration(uint64(math.MaxInt64))
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.EqualValues(t, s.Duration(), 0)
	assert.Equal(t, tsMalformed(&info.SpansMalformed{InvalidDuration: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeTypePassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	before := s.Type()
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, before, s.Type())
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeTypeTooLongV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetType(strings.Repeat("sql", 1000))
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, tsMalformed(&info.SpansMalformed{TypeTruncate: *atomic.NewInt64(1)}), ts)
}

func TestNormalizeServiceTagV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetService("retargeting(api-Staging ")
	s.SetStringAttribute(peerServiceKey, "retargeting(api-Peer ")
	s.SetStringAttribute(baseServiceKey, "retargeting(api-Base ")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, "retargeting_api-staging", s.Service())
	peerSvc, _ := s.GetAttributeAsString(peerServiceKey)
	baseSvc, _ := s.GetAttributeAsString(baseServiceKey)
	assert.Equal(t, "retargeting_api-peer", peerSvc)
	assert.Equal(t, "retargeting_api-base", baseSvc)
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeEnvV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()
	s := newTestSpanV1(idx.NewStringTable())
	s.SetEnv("123DEVELOPMENT")
	assert.NoError(t, a.normalizeV1(ts, s))
	assert.Equal(t, "123development", s.Env())
	assert.Equal(t, newTagStats(), ts)
}

func TestNormalizeSpanLinkNameV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	ts := newTagStats()

	// Normalize a span that contains an empty link name
	emptyLinkNameSpan := newTestSpanV1(idx.NewStringTable())
	emptyLinkNameSpan.Links()[0].SetStringAttribute("link.name", "")
	assert.NoError(t, a.normalizeV1(ts, emptyLinkNameSpan))
	linkName, _ := emptyLinkNameSpan.Links()[0].GetAttributeAsString("link.name")
	assert.Equal(t, linkName, normalize.DefaultSpanName)

	// Normalize a span that contains an invalid link name
	invalidLinkNameSpan := newTestSpanV1(idx.NewStringTable())
	invalidLinkNameSpan.Links()[0].SetStringAttribute("link.name", "!@#$%^&*()_+")
	assert.NoError(t, a.normalizeV1(ts, invalidLinkNameSpan))
	linkName, _ = invalidLinkNameSpan.Links()[0].GetAttributeAsString("link.name")
	assert.Equal(t, linkName, normalize.DefaultSpanName)

	// Normalize a span that contains a valid link name
	validLinkNameSpan := newTestSpanV1(idx.NewStringTable())
	validLinkNameSpan.Links()[0].SetStringAttribute("link.name", "valid_name")
	assert.NoError(t, a.normalizeV1(ts, validLinkNameSpan))
	linkName, _ = validLinkNameSpan.Links()[0].GetAttributeAsString("link.name")
	assert.Equal(t, linkName, "valid_name")
}
