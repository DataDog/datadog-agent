// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	_ "embed"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// create a new config to access default config values
var defaultAgentConfig = config.New()

//go:embed testdata/v04SpanEvents.msgp
var rubySpanEventsPayload []byte

func TestTraces(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("passthrough", func(t *testing.T) {
		if err := r.RunAgent([]byte("apm_config:\r\n  env: my-env")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(2, &testutil.TraceConfig{
			MinSpans: 3,
			Keep:     true,
		}, &testutil.SpanConfig{NumSpanEvents: 1})
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			if v.Env != "my-env" {
				t.Fatalf("Expected env my-env, got: %q", v.Env)
			}
			payloadsEqual(t, p, v)
		})
	})

	t.Run("reject", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(5, &testutil.TraceConfig{
			MinSpans: 3,
			Keep:     true,
		}, nil)
		for i := 0; i < 2; i++ {
			// user reject two traces
			// unset any error so they aren't grabbed by error sampler
			for _, span := range p[i] {
				span.Metrics["_sampling_priority_v1"] = -1
				span.Error = 0
			}
		}
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadsEqual(t, p[2:], v)
		})
	})

	t.Run("ignore_resources", func(t *testing.T) {
		if err := r.RunAgent([]byte(`apm_config:
  ignore_resources: ["GET /healthcheck/11"]`)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(5, &testutil.TraceConfig{
			MinSpans: 5,
			Keep:     true,
		}, nil)
		for _, span := range p[2] {
			span.Resource = "GET /healthcheck/11"
		}
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadsEqual(t, slices.Delete(p, 2, 3), v)
		})
	})

	t.Run("filter_tags", func(t *testing.T) {
		if err := r.RunAgent([]byte(`apm_config:
  filter_tags:
    require: ["env:prod", "db:mysql"]
    reject: ["outcome:success"]
  filter_tags_regex:
    require: ["env:^prod[0-9]{1}$", "priority:^high$"]
    reject: ["outcome:^success[0-9]{1}$", "bad-key:^bad-value$"]`)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(4, &testutil.TraceConfig{
			MinSpans: 4,
			Keep:     true,
		}, nil)
		for _, span := range p[0] {
			span.Meta = map[string]string{
				"env":      "prod",
				"db":       "mysql",
				"priority": "high",
			}
		}
		for _, span := range p[1] {
			span.Meta = map[string]string{
				"env":      "prod",
				"db":       "mysql",
				"priority": "high",
				"outcome":  "success1",
			}
		}
		for _, span := range p[2] {
			span.Meta = map[string]string{
				"env":      "prod",
				"db":       "mysql",
				"priority": "high",
				"outcome":  "success",
			}
		}
		for _, span := range p[3] {
			span.Meta = map[string]string{
				"env":      "prod",
				"db":       "mysql",
				"priority": "high",
				"bad-key":  "bad-value",
			}
		}
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadsEqual(t, p[:2], v)
		})
	})

	t.Run("normalize, obfuscate", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(1, &testutil.TraceConfig{
			MinSpans: 4,
			Keep:     true,
		}, nil)
		for _, span := range p[0] {
			span.Service = strings.Repeat("a", 200) // Too long
			span.Name = strings.Repeat("b", 200)    // Too long
		}
		p[0][0].Type = "sql"
		p[0][0].Resource = "SELECT secret FROM users WHERE id = 123"
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			tp := v.IdxTracerPayloads[0]
			strs := tp.Strings
			assert.Equal(t, "SELECT secret FROM users WHERE id = ?", idxStr(strs, tp.Chunks[0].Spans[0].ResourceRef))
			for _, s := range tp.Chunks[0].Spans {
				assert.Len(t, idxStr(strs, s.ServiceRef), 100)
				assert.Len(t, idxStr(strs, s.NameRef), 100)
			}
		})
	})

	t.Run("normalize, obfuscate, sqllexer", func(t *testing.T) {
		if err := r.RunAgent([]byte("apm_config:\r\n  features: [\"sqllexer\"]\r\n")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(1, &testutil.TraceConfig{
			MinSpans: 4,
			Keep:     true,
		}, nil)
		for _, span := range p[0] {
			span.Service = strings.Repeat("a", 200) // Too long
			span.Name = strings.Repeat("b", 200)    // Too long
		}
		p[0][0].Type = "sql"
		p[0][0].Resource = "SELECT secret FROM users WHERE id = 123"
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			tp := v.IdxTracerPayloads[0]
			strs := tp.Strings
			assert.Equal(t, "SELECT secret FROM users WHERE id = ?", idxStr(strs, tp.Chunks[0].Spans[0].ResourceRef))
			for _, s := range tp.Chunks[0].Spans {
				assert.Len(t, idxStr(strs, s.ServiceRef), 100)
				assert.Len(t, idxStr(strs, s.NameRef), 100)
			}
		})
	})

	t.Run("probabilistic", func(t *testing.T) {
		if err := r.RunAgent([]byte("apm_config:\r\n  probabilistic_sampler:\r\n    enabled: true\r\n    sampling_percentage: 100\r\n")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(2, &testutil.TraceConfig{
			MinSpans: 3,
			Keep:     true,
		}, nil)
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			payloadsEqual(t, p, v)
		})
	})

	t.Run("ruby-span-events", func(t *testing.T) {
		if err := r.RunAgent([]byte("apm_config:\r\n  env: my-env")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		if err := r.PostBinary("/v0.4/traces", rubySpanEventsPayload); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			tp := v.IdxTracerPayloads[0]
			strs := tp.Strings
			events := tp.Chunks[0].Spans[0].Events
			assert.Len(t, events, 1)
			spanEvent := events[0]
			assert.Equal(t, "event-name", idxStr(strs, spanEvent.NameRef))
			assert.NotZero(t, spanEvent.Time)
			assert.Len(t, spanEvent.Attributes, 1)
			val, ok := idxStrAttr(strs, spanEvent.Attributes, "key")
			assert.True(t, ok, "span event missing key attribute")
			assert.Equal(t, "value", val)
		})
	})
}

// payloadsEqual validates that the traces in from are the same as the ones in to.
// The convert-traces feature is enabled by default, so to carries its tracer
// payloads in the v1 string-indexed idx format.
func payloadsEqual(t *testing.T, from pb.Traces, to *pb.AgentPayload) {
	got := 0
	for _, tracerPayload := range to.IdxTracerPayloads {
		got += len(tracerPayload.Chunks)
	}
	if want := len(from); want != got {
		t.Fatalf("Expected %d traces, got %d", want, got)
	}
	var found int
	for _, t1 := range from {
		for _, tracerPayload := range to.IdxTracerPayloads {
			for _, t2 := range tracerPayload.Chunks {
				if tracesEqual(t1, tracerPayload.Strings, t2) {
					found++
					break
				}
			}
		}
	}
	if found != len(from) {
		t.Fatalf("Failed to match traces")
	}
	// validate the reported sampling configuration
	assert.Equal(t, to.TargetTPS, defaultAgentConfig.TargetTPS)
	assert.Equal(t, to.ErrorTPS, defaultAgentConfig.ErrorTPS)
	assert.Equal(t, to.RareSamplerEnabled, defaultAgentConfig.RareSamplerEnabled)
}

// tracesEqual reports whether the legacy-format trace from and the indexed chunk
// to are equal traces. The latter is allowed to contain additional tags.
func tracesEqual(from pb.Trace, toStrings []string, to *idx.TraceChunk) bool {
	if want, got := len(from), len(to.Spans); want != got {
		return false
	}
	// The trace ID is carried at the chunk level in the idx format; its low 64
	// bits are the legacy uint64 trace ID.
	if len(from) > 0 && len(to.TraceID) == 16 {
		if from[0].TraceID != binary.BigEndian.Uint64(to.TraceID[8:]) {
			return false
		}
	}
	var found int
	for _, s1 := range from {
		for _, s2 := range to.Spans {
			if spansEqual(s1, toStrings, s2) {
				found++
				break
			}
		}
	}
	return found == len(from)
}

// spansEqual reports whether the legacy-format span s1 and the indexed span s2
// are equal spans. s2 is permitted to have tags not present in s1, but not the
// other way around.
func spansEqual(s1 *pb.Span, toStrings []string, s2 *idx.Span) bool {
	if s1.Name != idxStr(toStrings, s2.NameRef) ||
		s1.Service != idxStr(toStrings, s2.ServiceRef) ||
		// obfuscation changes resource so we leave it out
		// s1.Resource != idxStr(toStrings, s2.ResourceRef) ||
		s1.SpanID != s2.SpanID ||
		s1.ParentID != s2.ParentID ||
		s1.Start != int64(s2.Start) ||
		s1.Duration != int64(s2.Duration) ||
		(s1.Error != 0) != s2.Error ||
		s1.Type != idxStr(toStrings, s2.TypeRef) {
		return false
	}
	for k, v := range s1.Meta {
		// env/version/component are promoted out of the attribute map into
		// dedicated span fields during conversion.
		switch k {
		case "env":
			if idxStr(toStrings, s2.EnvRef) != v {
				return false
			}
			continue
		case "version":
			if idxStr(toStrings, s2.VersionRef) != v {
				return false
			}
			continue
		case "component":
			if idxStr(toStrings, s2.ComponentRef) != v {
				return false
			}
			continue
		case "credit_card_number":
			// credit card obfuscation may rewrite this value; tests that care
			// about the obfuscated value assert on it explicitly.
			if !idxHasAttr(toStrings, s2.Attributes, k) {
				return false
			}
			continue
		}
		if got, ok := idxStrAttr(toStrings, s2.Attributes, k); !ok || got != v {
			return false
		}
	}
	for k, v := range s1.Metrics {
		if got, ok := idxNumAttr(toStrings, s2.Attributes, k); !ok || got != v {
			return false
		}
	}
	if len(s1.SpanEvents) != len(s2.Events) {
		return false
	}
	for i, se := range s1.SpanEvents {
		if se.Name != idxStr(toStrings, s2.Events[i].NameRef) ||
			se.TimeUnixNano != s2.Events[i].Time {
			return false
		}
	}
	return true
}
