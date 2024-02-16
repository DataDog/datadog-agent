// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
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
	a := &Agent{conf: config.New()}
	s := testSpan()
	before := s.Resource
	a.Truncate(s)
	assert.Equal(t, before, s.Resource)
}

func TestTruncateLongResource(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	s.Resource = strings.Repeat("TOOLONG", 5000)
	a.Truncate(s)
	assert.Equal(t, 5000, len(s.Resource))
}

func TestTruncateMetricsPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	before := s.Metrics
	a.Truncate(s)
	assert.Equal(t, before, s.Metrics)
}

func TestTruncateMetricsKeyTooLong(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	key := strings.Repeat("TOOLONG", 1000)
	s.Metrics[key] = 42
	a.Truncate(s)
	for k := range s.Metrics {
		assert.True(t, len(k) < MaxMetricsKeyLen+4)
	}
}

func TestTruncateMetaPassThru(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	before := s.Meta
	a.Truncate(s)
	assert.Equal(t, before, s.Meta)
}

func TestTruncateMetaKeyTooLong(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	key := strings.Repeat("TOOLONG", 1000)
	s.Meta[key] = "foo"
	a.Truncate(s)
	for k := range s.Meta {
		assert.True(t, len(k) < MaxMetaKeyLen+4)
	}
}

func TestTruncateMetaValueTooLong(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := testSpan()
	val := strings.Repeat("TOOLONG", 25000)
	s.Meta["foo"] = val
	a.Truncate(s)
	for _, v := range s.Meta {
		assert.True(t, len(v) < MaxMetaValLen+4)
	}
}

func TestTruncateStructuredMetaTag(t *testing.T) {
	key := strings.Repeat("k", MaxMetaKeyLen+1)
	val := strings.Repeat("v", MaxMetaValLen+1)

	for _, structuredSuffix := range []string{
		"json",
		"protobuf",
		"msgpack",
	} {
		suffix := structuredSuffix
		t.Run(suffix, func(t *testing.T) {
			a := &Agent{conf: config.New()}
			s := testSpan()
			structuredTagName := fmt.Sprintf("_dd.%s.%s", key, suffix)
			notStructuredTagName := fmt.Sprintf("key.%s", suffix)
			s.Meta[structuredTagName] = val
			s.Meta[notStructuredTagName] = val
			a.Truncate(s)
			// The structured value must not be truncated.
			require.Equal(t, val, s.Meta[structuredTagName])
			// The non structured value must be truncated.
			require.Len(t, s.Meta[notStructuredTagName], MaxMetaValLen+3) // 3 is the length of "..." added by the truncator
			require.NotEqual(t, val, s.Meta[notStructuredTagName])
		})
	}
}

func TestTruncateResource(t *testing.T) {
	a := &Agent{conf: config.New()}
	t.Run("over", func(t *testing.T) {
		r, ok := a.TruncateResource("resource")
		assert.True(t, ok)
		assert.Equal(t, "resource", r)
	})

	t.Run("under", func(t *testing.T) {
		s := strings.Repeat("a", a.conf.MaxResourceLen)
		r, ok := a.TruncateResource(s + "extra string")
		assert.False(t, ok)
		assert.Equal(t, s, r)
	})
}
