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
	s.SetStringAttribute("foo", val)
	a.TruncateV1(s)
	actualVal, ok := s.GetAttributeAsString("foo")
	assert.True(t, ok)
	assert.Len(t, actualVal, MaxMetaValLen+3)
}

func TestTruncateAttributeValueStructuredKeyTooLongV1(t *testing.T) {
	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	val := strings.Repeat("TOOLONG", 25000)
	s.SetStringAttribute("_dd.protected_key.json", val)
	a.TruncateV1(s)
	actualVal, ok := s.GetAttributeAsString("_dd.protected_key.json")
	assert.True(t, ok)
	assert.Equal(t, actualVal, val)
}

// attributeKeysV1 returns the keys of the span's attributes.
func attributeKeysV1(s *idx.InternalSpan) map[string]*idx.AnyValue {
	keys := make(map[string]*idx.AnyValue, len(s.Attributes()))
	for k, v := range s.Attributes() {
		keys[s.Strings.Get(k)] = v
	}
	return keys
}

// TestTruncateNonStringAttributeKeysV1 checks that TruncateV1 truncates the keys of non-string
// attributes the same way Truncate truncates legacy Metrics keys.
func TestTruncateNonStringAttributeKeysV1(t *testing.T) {
	longKey := strings.Repeat("k", MaxMetricsKeyLen+1)
	truncatedKey := strings.Repeat("k", MaxMetricsKeyLen) + "..."
	atLimitKey := strings.Repeat("a", MaxMetricsKeyLen)
	structuredKey := "_dd." + strings.Repeat("s", MaxMetricsKeyLen) + ".json"
	truncatedStructuredKey := structuredKey[:MaxMetricsKeyLen] + "..."

	t.Run("legacy", func(t *testing.T) {
		a := &Agent{conf: config.New()}
		s := testSpan()
		s.Metrics[longKey] = 42
		s.Metrics[atLimitKey] = 43
		s.Metrics[structuredKey] = 44
		a.Truncate(s)
		assert.NotContains(t, s.Metrics, longKey)
		assert.Equal(t, 42.0, s.Metrics[truncatedKey])
		assert.Equal(t, 43.0, s.Metrics[atLimitKey])
		// Structured meta keys are only exempt in Meta: a long structured Metrics key is truncated.
		assert.NotContains(t, s.Metrics, structuredKey)
		assert.Equal(t, 44.0, s.Metrics[truncatedStructuredKey])
	})

	for name, value := range map[string]*idx.AnyValue{
		"double": {Value: &idx.AnyValue_DoubleValue{DoubleValue: 42}},
		"int":    {Value: &idx.AnyValue_IntValue{IntValue: 42}},
		"bool":   {Value: &idx.AnyValue_BoolValue{BoolValue: true}},
		"array":  {Value: &idx.AnyValue_ArrayValue{ArrayValue: &idx.ArrayValue{Values: []*idx.AnyValue{{Value: &idx.AnyValue_IntValue{IntValue: 1}}}}}},
		"map":    {Value: &idx.AnyValue_KeyValueList{KeyValueList: &idx.KeyValueList{}}},
	} {
		t.Run("v1/"+name, func(t *testing.T) {
			a := &Agent{conf: config.New()}
			s := newTestSpanV1(idx.NewStringTable())
			s.Attributes()[s.Strings.Add(longKey)] = value
			atLimitValue := &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: 43}}
			s.Attributes()[s.Strings.Add(atLimitKey)] = atLimitValue
			structuredValue := &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: 44}}
			s.Attributes()[s.Strings.Add(structuredKey)] = structuredValue
			a.TruncateV1(s)
			attrs := attributeKeysV1(s)
			assert.NotContains(t, attrs, longKey)
			assert.Same(t, value, attrs[truncatedKey])
			assert.Same(t, atLimitValue, attrs[atLimitKey])
			assert.NotContains(t, attrs, structuredKey)
			assert.Same(t, structuredValue, attrs[truncatedStructuredKey])
		})
	}

	t.Run("v1/bytes", func(t *testing.T) {
		// Bytes attributes hold legacy meta_struct entries, whose keys Truncate does not touch.
		a := &Agent{conf: config.New()}
		s := newTestSpanV1(idx.NewStringTable())
		value := &idx.AnyValue{Value: &idx.AnyValue_BytesValue{BytesValue: []byte("x")}}
		s.Attributes()[s.Strings.Add(longKey)] = value
		a.TruncateV1(s)
		assert.Same(t, value, attributeKeysV1(s)[longKey])
	})
}

// TestTruncatePromotedFieldsV1 checks that TruncateV1 truncates the component, version and env,
// which legacy spans hold in Meta, with the Meta value limit.
func TestTruncatePromotedFieldsV1(t *testing.T) {
	long := strings.Repeat("v", MaxMetaValLen+1)
	truncated := strings.Repeat("v", MaxMetaValLen) + "..."
	atLimit := strings.Repeat("a", MaxMetaValLen)

	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"too-long", long, truncated},
		{"at-limit", atLimit, atLimit},
		{"short", "1.2.3", "1.2.3"},
	} {
		t.Run("legacy/"+tc.name, func(t *testing.T) {
			a := &Agent{conf: config.New()}
			s := testSpan()
			s.Meta["component"] = tc.in
			s.Meta["version"] = tc.in
			s.Meta["env"] = tc.in
			a.Truncate(s)
			assert.Equal(t, tc.want, s.Meta["component"])
			assert.Equal(t, tc.want, s.Meta["version"])
			assert.Equal(t, tc.want, s.Meta["env"])
		})
		t.Run("v1/"+tc.name, func(t *testing.T) {
			a := &Agent{conf: config.New()}
			s := newTestSpanV1(idx.NewStringTable())
			s.SetComponent(tc.in)
			s.SetVersion(tc.in)
			s.SetEnv(tc.in)
			a.TruncateV1(s)
			assert.Equal(t, tc.want, s.Component())
			assert.Equal(t, tc.want, s.Version())
			assert.Equal(t, tc.want, s.Env())
		})
	}
}

// TestTruncateStringAttributesAtLimitV1 checks that string attribute keys and values exactly at
// the limits are left unchanged, as in Truncate.
func TestTruncateStringAttributesAtLimitV1(t *testing.T) {
	key := strings.Repeat("k", MaxMetaKeyLen)
	val := strings.Repeat("v", MaxMetaValLen)

	a := &Agent{conf: config.New()}
	s := newTestSpanV1(idx.NewStringTable())
	s.SetStringAttribute(key, val)
	a.TruncateV1(s)
	got, ok := s.GetAttributeAsString(key)
	require.True(t, ok)
	assert.Equal(t, val, got)

	legacy := testSpan()
	legacy.Meta[key] = val
	a.Truncate(legacy)
	assert.Equal(t, val, legacy.Meta[key])
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
