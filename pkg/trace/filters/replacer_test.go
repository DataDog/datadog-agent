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
