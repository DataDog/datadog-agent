// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func testSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   42,
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

func TestTruncateResourcePassThru(t *testing.T) {
	s := testSpan()
	before := s.Resource
	Truncate(s)
	assert.Equal(t, before, s.Resource)
}

func TestTruncateLongResource(t *testing.T) {
	s := testSpan()
	s.Resource = strings.Repeat("TOOLONG", 5000)
	Truncate(s)
	assert.Equal(t, 5000, len(s.Resource))
}

func TestTruncateMetricsPassThru(t *testing.T) {
	s := testSpan()
	before := s.Metrics
	Truncate(s)
	assert.Equal(t, before, s.Metrics)
}

func TestTruncateMetricsKeyTooLong(t *testing.T) {
	s := testSpan()
	key := strings.Repeat("TOOLONG", 1000)
	s.Metrics[key] = 42
	Truncate(s)
	for k := range s.Metrics {
		assert.True(t, len(k) < traceutil.MaxMetricsKeyLen+4)
	}
}

func TestTruncateMetaPassThru(t *testing.T) {
	s := testSpan()
	before := s.Meta
	Truncate(s)
	assert.Equal(t, before, s.Meta)
}

func TestTruncateMetaKeyTooLong(t *testing.T) {
	s := testSpan()
	key := strings.Repeat("TOOLONG", 1000)
	s.Meta[key] = "foo"
	Truncate(s)
	for k := range s.Meta {
		assert.True(t, len(k) < traceutil.MaxMetaKeyLen+4)
	}
}

func TestTruncateMetaValueTooLong(t *testing.T) {
	s := testSpan()
	val := strings.Repeat("TOOLONG", 5000)
	s.Meta["foo"] = val
	Truncate(s)
	for _, v := range s.Meta {
		assert.True(t, len(v) < traceutil.MaxMetaValLen+4)
	}
}
