// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	"math/rand/v2"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestReplacer(t *testing.T) {
	t.Run("stats", func(tt *testing.T) {
		for _, testCase := range []struct {
			rules     [][3]string
			got, want *pb.ClientGroupedStats
		}{
			{
				rules: [][3]string{
					{"http.status_code", "400", "200"},
					{"resource.name", "prod", "stage"},
					{"*", "123abc", "[REDACTED]"},
				},
				got: &pb.ClientGroupedStats{
					Resource:       "this is 123abc on prod",
					HTTPStatusCode: 400,
				},
				want: &pb.ClientGroupedStats{
					Resource:       "this is [REDACTED] on stage",
					HTTPStatusCode: 200,
				},
			},
			{
				rules: [][3]string{
					{"*", "200", "202"},
				},
				got: &pb.ClientGroupedStats{
					Resource:       "/code/200/profile",
					HTTPStatusCode: 200,
				},
				want: &pb.ClientGroupedStats{
					Resource:       "/code/202/profile",
					HTTPStatusCode: 202,
				},
			},
		} {
			tr := NewReplacer(parseRulesFromString(testCase.rules))
			tr.ReplaceStatsGroup(testCase.got)
			assert.Equal(tt, testCase.got, testCase.want)
		}
	})
	t.Run("span events", func(tt *testing.T) {
		for _, testCase := range []struct {
			rules     [][3]string
			got, want map[string]string
		}{
			{
				rules: [][3]string{
					{"http.url", "(token/)([^/]*)", "${1}?"},
					{"http.url", "guid", "[REDACTED]"},
					{"custom.tag", "(/foo/bar/).*", "${1}extra"},
					{"a", "b", "c"},
					{"some.num", "1", "one!"},
					{"some.dbl", "42.1", "42.5"},
					{"is.potato", "true", "false"},
					{"my.nums", "42", "100"},
				},
				got: map[string]string{
					"http.url":   "some/guid/token/abcdef/abc",
					"custom.tag": "/foo/bar/foo",
					"some.num":   "1",
					"some.dbl":   "42.1",
					"is.potato":  "true",
					"my.nums":    "42",
				},
				want: map[string]string{
					"http.url":   "some/[REDACTED]/token/?/abc",
					"custom.tag": "/foo/bar/extra",
					"some.num":   "one!",
					"some.dbl":   "42.5",
					"is.potato":  "false",
					"my.nums":    "100",
				},
			},
		} {
			rules := parseRulesFromString(testCase.rules)
			tr := NewReplacer(rules)

			// Create a new InternalTraceChunk
			chunk := &idx.InternalTraceChunk{
				Spans:   make([]*idx.InternalSpan, 1),
				Strings: idx.NewStringTable(),
			}

			// Create a span with a span event
			span := idx.NewInternalSpan(chunk.Strings, &idx.Span{
				SpanID:      rand.Uint64(),
				ParentID:    1111,
				ServiceRef:  chunk.Strings.Add("test-service"),
				NameRef:     chunk.Strings.Add("test-span"),
				ResourceRef: chunk.Strings.Add("test-resource"),
				Start:       1448466874000000000,
				Duration:    10000000,
				Attributes:  map[uint32]*idx.AnyValue{},
				Events: []*idx.SpanEvent{
					{
						Time:       0,
						NameRef:    chunk.Strings.Add("foo"),
						Attributes: map[uint32]*idx.AnyValue{},
					},
				},
			})

			// Set the span event attributes
			for k, v := range testCase.got {
				span.Events()[0].SetAttributeFromString(k, v)
			}
			chunk.Spans[0] = span

			// Apply the replacer
			tr.ReplaceV1(chunk)

			// Verify results
			events := chunk.Spans[0].Events()
			assert.Equal(tt, 1, len(events))
			for k, v := range testCase.want {
				if val, ok := events[0].GetAttributeAsString(k); ok {
					assert.Equal(tt, v, val)
				}
			}
		}
	})

	t.Run("traces v1", func(tt *testing.T) {
		for _, testCase := range []struct {
			rules     [][3]string
			got, want map[string]string
		}{
			{
				rules: [][3]string{
					{"http.url", "(token/)([^/]*)", "${1}?"},
					{"http.url", "guid", "[REDACTED]"},
					{"custom.tag", "(/foo/bar/).*", "${1}extra"},
					{"a", "b", "c"},
				},
				got: map[string]string{
					"http.url":   "some/guid/token/abcdef/abc",
					"custom.tag": "/foo/bar/foo",
				},
				want: map[string]string{
					"http.url":   "some/[REDACTED]/token/?/abc",
					"custom.tag": "/foo/bar/extra",
				},
			},
			{
				rules: [][3]string{
					{"*", "(token/)([^/]*)", "${1}?"},
					{"*", "this", "that"},
					{"http.url", "guid", "[REDACTED]"},
					{"custom.tag", "(/foo/bar/).*", "${1}extra"},
					{"resource.name", "prod", "stage"},
				},
				got: map[string]string{
					"resource.name": "this is prod",
					"http.url":      "some/[REDACTED]/token/abcdef/abc",
					"other.url":     "some/guid/token/abcdef/abc",
					"custom.tag":    "/foo/bar/foo",
					"_special":      "this should not be changed",
				},
				want: map[string]string{
					"resource.name": "that is stage",
					"http.url":      "some/[REDACTED]/token/?/abc",
					"other.url":     "some/guid/token/?/abc",
					"custom.tag":    "/foo/bar/extra",
					"_special":      "this should not be changed",
				},
			},
		} {
			rules := parseRulesFromString(testCase.rules)
			tr := NewReplacer(rules)

			// Create a new InternalTraceChunk
			chunk := &idx.InternalTraceChunk{
				Spans:   make([]*idx.InternalSpan, 1),
				Strings: idx.NewStringTable(),
			}

			// Create spans with the test data
			span := newTestSpanV1EmptyAttributes(chunk.Strings)
			for k, v := range testCase.got {
				if k == "resource.name" {
					span.SetResource(v)
				} else {
					span.SetAttributeFromString(k, v)
				}
			}
			chunk.Spans[0] = span

			// Apply the replacer
			tr.ReplaceV1(chunk)

			// Verify results
			for _, span := range chunk.Spans {
				for k, v := range testCase.want {
					switch k {
					case "resource.name":
						assert.Equal(tt, v, span.Resource())
					default:
						if val, ok := span.GetAttributeAsString(k); ok {
							assert.Equal(tt, v, val)
						}
					}
				}
			}
		}
	})
}

// TestReplacerWildcardEnv verifies that "*" rules never change the span env, which
// the trace agent can use as the env for sampling and stats (see traceutil.GetEnv),
// while they still apply to every other tag and to span event attributes.
func TestReplacerWildcardEnv(t *testing.T) {
	rules := parseRulesFromString([][3]string{{"*", "prod", "stage"}})

	t.Run("v0 meta", func(t *testing.T) {
		span := &pb.Span{
			Resource: "prod-resource",
			Meta:     map[string]string{"env": "prod", "custom.tag": "prod-value"},
			SpanEvents: []*pb.SpanEvent{{Attributes: map[string]*pb.AttributeAnyValue{
				"env": {Type: pb.AttributeAnyValue_STRING_VALUE, StringValue: "prod"},
			}}},
		}
		NewReplacer(rules).Replace(pb.Trace{span})
		assert.Equal(t, "prod", span.Meta["env"])
		assert.Equal(t, "stage-value", span.Meta["custom.tag"])
		assert.Equal(t, "stage-resource", span.Resource)
		assert.Equal(t, "stage", span.SpanEvents[0].Attributes["env"].StringValue, "span event attributes are not the span env")
	})

	t.Run("v0 numeric env metric is not moved to meta", func(t *testing.T) {
		// replaceNumericTag moves non-numeric results from Metrics to Meta.
		span := &pb.Span{
			Meta:    map[string]string{},
			Metrics: map[string]float64{"env": 1, "latency": 1},
		}
		NewReplacer(parseRulesFromString([][3]string{{"*", "1", "prod"}})).Replace(pb.Trace{span})
		assert.NotContains(t, span.Meta, "env")
		assert.Equal(t, float64(1), span.Metrics["env"])
		assert.Equal(t, "prod", span.Meta["latency"], "other metrics are still replaced")
	})

	t.Run("v1 env in attributes and env field", func(t *testing.T) {
		// Spans decoded from msgpack v0.4/v0.5 payloads (UnmarshalMsgConverted)
		// keep env both in the dedicated env field and in Attributes().
		strings := idx.NewStringTable()
		span := newTestSpanV1EmptyAttributes(strings)
		span.SetEnv("prod")
		envRef := strings.Add("env")
		span.Attributes()[envRef] = &idx.AnyValue{Value: &idx.AnyValue_StringValueRef{StringValueRef: strings.Add("prod")}}
		span.SetAttributeFromString("custom.tag", "prod-value")
		chunk := &idx.InternalTraceChunk{Strings: strings, Spans: []*idx.InternalSpan{span}}
		NewReplacer(rules).ReplaceV1(chunk)
		assert.Equal(t, "prod", span.Env())
		assert.Equal(t, "prod", span.Attributes()[envRef].AsString(strings))
		custom, _ := span.GetAttributeAsString("custom.tag")
		assert.Equal(t, "stage-value", custom)
	})

	t.Run("v1 env field only, span event env still replaced", func(t *testing.T) {
		// Spans converted with api.ConvertToIdx (JSON payloads, v0.7, serverless)
		// only have the dedicated env field, so the wildcard loop never sees env.
		// This guards that, and that span event attributes named env are replaced.
		strings := idx.NewStringTable()
		span := idx.NewInternalSpan(strings, &idx.Span{
			ResourceRef: strings.Add("prod-resource"),
			Attributes:  map[uint32]*idx.AnyValue{},
			Events: []*idx.SpanEvent{
				{NameRef: strings.Add("event"), Attributes: map[uint32]*idx.AnyValue{}},
			},
		})
		span.SetEnv("prod")
		span.Events()[0].SetAttributeFromString("env", "prod")
		chunk := &idx.InternalTraceChunk{Strings: strings, Spans: []*idx.InternalSpan{span}}
		NewReplacer(rules).ReplaceV1(chunk)
		assert.Equal(t, "prod", span.Env())
		assert.Equal(t, "stage-resource", span.Resource())
		eventEnv, _ := span.Events()[0].GetAttributeAsString("env")
		assert.Equal(t, "stage", eventEnv, "span event attributes are not the span env")
	})
}

// GetTestSpan returns a Span with different fields set
func newTestSpanV1EmptyAttributes(strings *idx.StringTable) *idx.InternalSpan {
	return idx.NewInternalSpan(strings, &idx.Span{
		SpanID:      rand.Uint64(),
		ParentID:    1111,
		ServiceRef:  strings.Add("django"),
		NameRef:     strings.Add("django.controller"),
		ResourceRef: strings.Add("GET /some/raclette"),
		Start:       1448466874000000000,
		Duration:    10000000,
		Attributes:  map[uint32]*idx.AnyValue{},
	})
}

func parseRulesFromString(rules [][3]string) []*config.ReplaceRule {
	r := make([]*config.ReplaceRule, 0, len(rules))
	for _, rule := range rules {
		key, re, str := rule[0], rule[1], rule[2]
		r = append(r, &config.ReplaceRule{
			Name:    key,
			Pattern: re,
			Re:      regexp.MustCompile(re),
			Repl:    str,
		})
	}
	return r
}

// replaceFilterTestSpan creates a span from a list of tags and uses
// special tag names (e.g. resource.name) to target attributes.
func replaceFilterTestSpan(tags map[string]string) *pb.Span {
	span := &pb.Span{Meta: make(map[string]string)}
	for k, v := range tags {
		switch k {
		case "resource.name":
			span.Resource = v
		default:
			span.Meta[k] = v
		}
	}
	return span
}

// TestReplaceFilterTestSpan tests the replaceFilterTestSpan test
// helper function.
func TestReplaceFilterTestSpan(t *testing.T) {
	for _, tt := range []struct {
		tags map[string]string
		want *pb.Span
	}{
		{
			tags: map[string]string{
				"resource.name": "a",
				"http.url":      "url",
				"custom.tag":    "val",
			},
			want: &pb.Span{
				Resource: "a",
				Meta: map[string]string{
					"http.url":   "url",
					"custom.tag": "val",
				},
			},
		},
	} {
		got := replaceFilterTestSpan(tt.tags)
		assert.Equal(t, tt.want, got)
	}
}

// TestReplaceAttributeAnyValue_NilArrayAndElement ensures the replacer tolerates
// an ARRAY_VALUE attribute with a nil array and with a nil element (both valid
// msgpack decode results) without panicking.
func TestReplaceAttributeAnyValue_NilArrayAndElement(t *testing.T) {
	f := Replacer{}
	re := regexp.MustCompile("secret")

	assert.NotPanics(t, func() {
		val := &pb.AttributeAnyValue{Type: pb.AttributeAnyValue_ARRAY_VALUE, ArrayValue: nil}
		assert.Same(t, val, f.replaceAttributeAnyValue(re, val, "?"))
	})

	assert.NotPanics(t, func() {
		val := &pb.AttributeAnyValue{
			Type:       pb.AttributeAnyValue_ARRAY_VALUE,
			ArrayValue: &pb.AttributeArray{Values: []*pb.AttributeArrayValue{nil}},
		}
		got := f.replaceAttributeAnyValue(re, val, "?")
		assert.Nil(t, got.ArrayValue.Values[0])
	})
}
