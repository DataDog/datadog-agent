// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"bytes"
	_ "embed"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestTracesV1(t *testing.T) {
	var r test.Runner
	r.Verbose = true
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("passthrough", func(t *testing.T) {
		if err := r.RunAgent([]byte("log_level: debug\r\napm_config:\r\n  env: my-env")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayloadV1(2, &testutil.TraceConfig{
			MinSpans: 3,
			Keep:     true,
		}, &testutil.SpanConfig{NumSpanEvents: 1})

		if err := r.PostV1(p); err != nil {
			t.Fatal(err)
		}

		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			if v.Env != "my-env" {
				t.Fatalf("Expected env my-env, got: %q", v.Env)
			}
			payloadsEqualV1(t, p, v)
		})
	})

	t.Run("reject", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayloadV1(3, &testutil.TraceConfig{
			MinSpans: 2,
			Keep:     true,
		}, nil)
		for i := 0; i < 2; i++ {
			// user reject two traces
			p.Chunks[i].Priority = -1
			// unset any error so they aren't grabbed by error sampler
			for _, span := range p.Chunks[i].Spans {
				span.SetError(false)
			}
		}
		if err := r.PostV1(p); err != nil {
			t.Fatal(err)
		}
		p.Chunks = p.Chunks[2:] // We expect the first two chunks to be dropped
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadsEqualV1(t, p, v)
		})
	})

	t.Run("ignore_resources", func(t *testing.T) {
		if err := r.RunAgent([]byte(`apm_config:
  ignore_resources: ["GET /healthcheck/11"]`)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayloadV1(5, &testutil.TraceConfig{
			MinSpans: 5,
			Keep:     true,
		}, nil)
		for _, span := range p.Chunks[2].Spans {
			span.SetResource("GET /healthcheck/11")
		}
		if err := r.PostV1(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			p.Chunks = slices.Delete(p.Chunks, 2, 3)
			payloadsEqualV1(t, p, v)
		})
	})

	// 	t.Run("filter_tags", func(t *testing.T) {
	// 		if err := r.RunAgent([]byte(`apm_config:
	//   filter_tags:
	//     require: ["env:prod", "db:mysql"]
	//     reject: ["outcome:success"]
	//   filter_tags_regex:
	//     require: ["env:^prod[0-9]{1}$", "priority:^high$"]
	//     reject: ["outcome:^success[0-9]{1}$", "bad-key:^bad-value$"]`)); err != nil {
	// 			t.Fatal(err)
	// 		}
	// 		defer r.KillAgent()

	// 		p := testutil.GeneratePayload(4, &testutil.TraceConfig{
	// 			MinSpans: 4,
	// 			Keep:     true,
	// 		}, nil)
	// 		for _, span := range p[0] {
	// 			span.Meta = map[string]string{
	// 				"env":      "prod",
	// 				"db":       "mysql",
	// 				"priority": "high",
	// 			}
	// 		}
	// 		for _, span := range p[1] {
	// 			span.Meta = map[string]string{
	// 				"env":      "prod",
	// 				"db":       "mysql",
	// 				"priority": "high",
	// 				"outcome":  "success1",
	// 			}
	// 		}
	// 		for _, span := range p[2] {
	// 			span.Meta = map[string]string{
	// 				"env":      "prod",
	// 				"db":       "mysql",
	// 				"priority": "high",
	// 				"outcome":  "success",
	// 			}
	// 		}
	// 		for _, span := range p[3] {
	// 			span.Meta = map[string]string{
	// 				"env":      "prod",
	// 				"db":       "mysql",
	// 				"priority": "high",
	// 				"bad-key":  "bad-value",
	// 			}
	// 		}
	// 		if err := r.Post(p); err != nil {
	// 			t.Fatal(err)
	// 		}
	// 		waitForTrace(t, &r, func(v *pb.AgentPayload) {
	// 			payloadsEqual(t, p[:2], v)
	// 		})
	// 	})

	t.Run("normalize, obfuscate", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayloadV1(1, &testutil.TraceConfig{
			MinSpans: 4,
			Keep:     true,
		}, nil)
		for _, span := range p.Chunks[0].Spans {
			span.SetService(strings.Repeat("a", 200)) // Too long
			span.SetName(strings.Repeat("b", 200))    // Too long
		}
		p.Chunks[0].Spans[0].SetType("sql")
		p.Chunks[0].Spans[0].SetResource("SELECT secret FROM users WHERE id = 123")
		if err := r.PostV1(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadStrings := v.IdxTracerPayloads[0].Strings
			assert.NotContains(t, payloadStrings, "SELECT secret FROM users WHERE id = 123")
			assert.Equal(t, "SELECT secret FROM users WHERE id = ?", payloadStrings[v.IdxTracerPayloads[0].Chunks[0].Spans[0].ResourceRef])
			for _, s := range v.IdxTracerPayloads[0].Chunks[0].Spans {
				assert.Len(t, payloadStrings[s.ServiceRef], 100)
				assert.Len(t, payloadStrings[s.NameRef], 100)
			}
		})
	})
}

// payloadsEqual validates that the traces in from are the same as the ones in to.
func payloadsEqualV1(t *testing.T, from *idx.InternalTracerPayload, to *pb.AgentPayload) {
	got := 0
	for _, tracerPayload := range to.IdxTracerPayloads {
		got += len(tracerPayload.Chunks)
	}
	if want := len(from.Chunks); want != got {
		t.Fatalf("Expected %d traces, got %d", want, got)
	}

	var found int
	for _, t1 := range from.Chunks {
		for _, tracerPayload := range to.IdxTracerPayloads {
			for _, t2 := range tracerPayload.Chunks {
				if tracesEqualV1(t1, tracerPayload.Strings, t2) {
					found++
					break
				}
			}
		}
	}
	if found != len(from.Chunks) {
		t.Fatalf("Failed to match traces")
	}
	// validate the reported sampling configuration
	assert.Equal(t, to.TargetTPS, defaultAgentConfig.TargetTPS)
	assert.Equal(t, to.ErrorTPS, defaultAgentConfig.ErrorTPS)
	assert.Equal(t, to.RareSamplerEnabled, defaultAgentConfig.RareSamplerEnabled)
}

// tracesEqual reports whether from and to are equal traces. The latter is allowed
// to contain additional tags.
func tracesEqualV1(from *idx.InternalTraceChunk, toStrings []string, to *idx.TraceChunk) bool {
	if want, got := len(from.Spans), len(to.Spans); want != got {
		return false
	}
	if from.Priority != to.Priority {
		return false
	}
	if from.Origin() != toStrings[to.OriginRef] {
		return false
	}
	if from.DecisionMaker() != toStrings[to.DecisionMakerRef] {
		return false
	}
	if from.DroppedTrace != to.DroppedTrace {
		return false
	}
	if !bytes.Equal(from.TraceID, to.TraceID) {
		return false
	}
	toAttrs := make(map[string]string)
	for key, attr := range to.Attributes {
		tempToStrings := idx.StringTableFromArray(toStrings)
		toAttrs[toStrings[key]] = attr.AsString(tempToStrings)
	}
	for kFrom, vFrom := range from.Attributes {
		if toVal, ok := toAttrs[from.Strings.Get(kFrom)]; ok {
			if toVal != vFrom.AsString(from.Strings) {
				return false
			}
			// value matched, continue to next attribute
			continue
		}
		// value not found, return false
		return false
	}
	var found int
	for _, s1 := range from.Spans {
		for _, s2 := range to.Spans {
			if spansEqualV1(s1, toStrings, s2) {
				found++
				break
			}
		}
	}
	return found == len(from.Spans)
}

// spansEqual reports whether s1 and s2 are equal spans. s2 is permitted to have
// tags not present in s1, but not the other way around.
func spansEqualV1(s1 *idx.InternalSpan, toStrings []string, s2 *idx.Span) bool {
	if s1.Name() != toStrings[s2.NameRef] {
		return false
	}
	if s1.Service() != toStrings[s2.ServiceRef] {
		return false
	}
	// obfuscation changes resource so we leave it out
	// if s1.Resource() != s2.Resource {
	// 	return false
	// }
	if s1.SpanID() != s2.SpanID {
		return false
	}
	if s1.ParentID() != s2.ParentID {
		return false
	}
	if s1.Start() != s2.Start {
		return false
	}
	if s1.Name() != toStrings[s2.NameRef] {
		return false
	}
	if s1.Duration() != s2.Duration {
		return false
	}
	if s1.Error() != s2.Error {
		return false
	}
	if s1.Type() != toStrings[s2.TypeRef] {
		return false
	}
	if s1.Env() != toStrings[s2.EnvRef] {
		return false
	}
	if s1.Kind() != s2.Kind {
		return false
	}
	if s1.Version() != toStrings[s2.VersionRef] {
		return false
	}
	if s1.Component() != toStrings[s2.ComponentRef] {
		return false
	}
	// TODO check attributes
	// for k := range s1.Meta {
	// 	if _, ok := s2.Meta[k]; !ok {
	// 		return false
	// 	}
	// }
	// for k := range s1.Metrics {
	// 	if _, ok := s2.Metrics[k]; !ok {
	// 		return false
	// 	}
	// }
	// if len(s1.SpanEvents) != len(s2.SpanEvents) {
	// 	return false
	// }
	// for i, se := range s1.SpanEvents {
	// 	if se.Name != s2.SpanEvents[i].Name ||
	// 		se.TimeUnixNano != s2.SpanEvents[i].TimeUnixNano {
	// 		return false
	// 	}
	// }
	return true
}
