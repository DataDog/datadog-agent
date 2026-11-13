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
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
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
			notStructuredTagName := "key." + suffix
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

func TestTruncateResourcePassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	before := s.Resource()
	a.TruncateV1(s)
	assert.Equal(t, before, s.Resource())
}

func TestTruncateLongResourceV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	s.SetResource(strings.Repeat("TOOLONG", 5000))
	a.TruncateV1(s)
	assert.Equal(t, 5000, len(s.Resource()))
}

func TestTruncateAttributesPassThruV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	before := make(map[string]string)
	for k, v := range s.Attributes() {
		if strVal, ok := v.Value.(*idx.AnyValue_StringValueRef); ok {
			before[s.Strings.Get(k)] = s.Strings.Get(strVal.StringValueRef)
		}
	}
	a.TruncateV1(s)
	after := make(map[string]string)
	for k, v := range s.Attributes() {
		if strVal, ok := v.Value.(*idx.AnyValue_StringValueRef); ok {
			after[s.Strings.Get(k)] = s.Strings.Get(strVal.StringValueRef)
		}
	}
	assert.Equal(t, before, after)
}

func TestTruncateAttributeKeyTooLongV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	key := strings.Repeat("TOOLONG", 1000)
	s.SetStringAttribute(key, "foo")
	a.TruncateV1(s)
	for k := range s.Attributes() {
		assert.True(t, len(s.Strings.Get(k)) < MaxMetaKeyLen+4, "key %s is too long, len is %d", s.Strings.Get(k), len(s.Strings.Get(k)))
	}
}

func TestTruncateAttributeValueTooLongV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	val := strings.Repeat("TOOLONG", 25000)
	atLimitKey := strings.Repeat("k", MaxMetaKeyLen)
	atLimitVal := strings.Repeat("v", MaxMetaValLen)
	s.SetStringAttribute("foo", val)
	s.SetStringAttribute(atLimitKey, atLimitVal)
	// Legacy spans hold these in Meta, so they get the Meta value limit.
	s.SetEnv(val)
	s.SetComponent(val)
	s.SetVersion(atLimitVal)
	a.TruncateV1(s)
	actualVal, ok := s.GetAttributeAsString("foo")
	assert.True(t, ok)
	assert.Len(t, actualVal, MaxMetaValLen+3)
	actualVal, ok = s.GetAttributeAsString(atLimitKey)
	assert.True(t, ok)
	assert.Equal(t, atLimitVal, actualVal)
	assert.Equal(t, val[:MaxMetaValLen]+"...", s.Env())
	assert.Equal(t, val[:MaxMetaValLen]+"...", s.Component())
	assert.Equal(t, atLimitVal, s.Version())
}

// TestTruncateStructuredMetaTagV1 mirrors TestTruncateStructuredMetaTag: string attributes with
// a structured meta key keep their key and value, others are truncated.
func TestTruncateStructuredMetaTagV1(t *testing.T) {
	key := strings.Repeat("k", MaxMetaKeyLen+1)
	val := strings.Repeat("v", MaxMetaValLen+1)

	for _, suffix := range []string{"json", "protobuf", "msgpack"} {
		t.Run(suffix, func(t *testing.T) {
			a := &Agent{conf: config.New()}
			s := newTestSpanV1(idx.NewStringTable())
			structuredTagName := fmt.Sprintf("_dd.%s.%s", key, suffix)
			notStructuredTagName := "key." + suffix
			s.SetStringAttribute(structuredTagName, val)
			s.SetStringAttribute(notStructuredTagName, val)
			a.TruncateV1(s)
			got, ok := s.GetAttributeAsString(structuredTagName)
			require.True(t, ok)
			require.Equal(t, val, got)
			got, ok = s.GetAttributeAsString(notStructuredTagName)
			require.True(t, ok)
			require.Len(t, got, MaxMetaValLen+3)
		})
	}
}

// TestTruncateNonStringAttributeKeysV1 checks that TruncateV1 truncates the keys of non-string
// attributes the same way Truncate truncates legacy Metrics keys.
func TestTruncateNonStringAttributeKeysV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	attrs := s.Attributes()
	// Each long key uses its own character so that the truncated keys do not collide.
	values := map[string]*idx.AnyValue{
		"d": {Value: &idx.AnyValue_DoubleValue{DoubleValue: 42}},
		"i": {Value: &idx.AnyValue_IntValue{IntValue: 42}},
		"b": {Value: &idx.AnyValue_BoolValue{BoolValue: true}},
		"a": {Value: &idx.AnyValue_ArrayValue{ArrayValue: &idx.ArrayValue{Values: []*idx.AnyValue{{Value: &idx.AnyValue_IntValue{IntValue: 1}}}}}},
		"m": {Value: &idx.AnyValue_KeyValueList{KeyValueList: &idx.KeyValueList{}}},
	}
	for c, v := range values {
		attrs[s.Strings.Add(strings.Repeat(c, MaxMetricsKeyLen+1))] = v
	}
	// Bytes attributes hold legacy meta_struct entries, whose keys Truncate does not touch.
	bytesKey := strings.Repeat("y", MaxMetricsKeyLen+1)
	bytesValue := &idx.AnyValue{Value: &idx.AnyValue_BytesValue{BytesValue: []byte("x")}}
	attrs[s.Strings.Add(bytesKey)] = bytesValue
	atLimitKey := strings.Repeat("l", MaxMetricsKeyLen)
	atLimitValue := &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: 43}}
	attrs[s.Strings.Add(atLimitKey)] = atLimitValue
	// Structured meta keys are only exempt in Meta: a long structured Metrics key is truncated.
	structuredKey := "_dd." + strings.Repeat("s", MaxMetricsKeyLen) + ".json"
	structuredValue := &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: 44}}
	attrs[s.Strings.Add(structuredKey)] = structuredValue

	a.TruncateV1(s)

	got := make(map[string]*idx.AnyValue, len(s.Attributes()))
	for k, v := range s.Attributes() {
		got[s.Strings.Get(k)] = v
	}
	for c, v := range values {
		assert.NotContains(t, got, strings.Repeat(c, MaxMetricsKeyLen+1))
		assert.Same(t, v, got[strings.Repeat(c, MaxMetricsKeyLen)+"..."], c)
	}
	assert.Same(t, bytesValue, got[bytesKey])
	assert.Same(t, atLimitValue, got[atLimitKey])
	assert.NotContains(t, got, structuredKey)
	assert.Same(t, structuredValue, got[structuredKey[:MaxMetricsKeyLen]+"..."])
}
