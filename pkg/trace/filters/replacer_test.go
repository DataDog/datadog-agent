// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func TestReplacer(t *testing.T) {
	t.Run("traces", func(tt *testing.T) {
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
			root := replaceFilterTestSpan(testCase.got)
			childSpan := replaceFilterTestSpan(testCase.got)
			trace := pb.Trace{root, childSpan}
			tr.Replace(trace)
			for k, v := range testCase.want {
				switch k {
				case "resource.name":
					// test that the filter applies to all spans, not only the root
					assert.Equal(tt, v, root.Resource)
					assert.Equal(tt, v, childSpan.Resource)
				default:
					assert.Equal(tt, v, root.Meta[k])
					assert.Equal(tt, v, childSpan.Meta[k])
				}
			}
		}
	})

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
			got, want map[string]*pb.AttributeAnyValue
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
				got: map[string]*pb.AttributeAnyValue{
					"http.url": {
						Type:        pb.AttributeAnyValue_STRING_VALUE,
						StringValue: "some/guid/token/abcdef/abc",
					},
					"custom.tag": {
						Type:        pb.AttributeAnyValue_STRING_VALUE,
						StringValue: "/foo/bar/foo",
					},
					"some.num": {
						Type:     pb.AttributeAnyValue_INT_VALUE,
						IntValue: 1,
					},
					"some.dbl": {
						Type:        pb.AttributeAnyValue_DOUBLE_VALUE,
						DoubleValue: 42.1,
					},
					"is.potato": {
						Type:      pb.AttributeAnyValue_BOOL_VALUE,
						BoolValue: true,
					},
					"my.nums": {
						Type: pb.AttributeAnyValue_ARRAY_VALUE,
						ArrayValue: &pb.AttributeArray{
							Values: []*pb.AttributeArrayValue{
								{
									Type:     pb.AttributeArrayValue_INT_VALUE,
									IntValue: 123,
								},
								{
									Type:     pb.AttributeArrayValue_INT_VALUE,
									IntValue: 42,
								},
							},
						},
					},
				},
				want: map[string]*pb.AttributeAnyValue{
					"http.url": {
						Type:        pb.AttributeAnyValue_STRING_VALUE,
						StringValue: "some/[REDACTED]/token/?/abc",
					},
					"custom.tag": {
						Type:        pb.AttributeAnyValue_STRING_VALUE,
						StringValue: "/foo/bar/extra",
					},
					"some.num": {
						Type:        pb.AttributeAnyValue_STRING_VALUE,
						StringValue: "one!",
					},
					"some.dbl": {
						Type:        pb.AttributeAnyValue_DOUBLE_VALUE,
						DoubleValue: 42.5,
					},
					"is.potato": {
						Type:      pb.AttributeAnyValue_BOOL_VALUE,
						BoolValue: false,
					},
					"my.nums": {
						Type: pb.AttributeAnyValue_ARRAY_VALUE,
						ArrayValue: &pb.AttributeArray{
							Values: []*pb.AttributeArrayValue{
								{
									Type:     pb.AttributeArrayValue_INT_VALUE,
									IntValue: 123,
								},
								{
									Type:     pb.AttributeArrayValue_INT_VALUE,
									IntValue: 100,
								},
							},
						},
					},
				},
			},
		} {
			rules := parseRulesFromString(testCase.rules)
			tr := NewReplacer(rules)
			root := replaceFilterTestSpanEvent(testCase.got)
			trace := pb.Trace{root}
			tr.Replace(trace)
			for k, v := range testCase.want {
				assert.Equal(tt, v, root.SpanEvents[0].Attributes[k])
			}
		}
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

// replaceFilterTestSpan creates a span with a span event with the provided attributes
func replaceFilterTestSpanEvent(attributes map[string]*pb.AttributeAnyValue) *pb.Span {
	span := &pb.Span{SpanEvents: []*pb.SpanEvent{
		{
			TimeUnixNano: 0,
			Name:         "foo",
			Attributes:   attributes,
		},
	}}
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
