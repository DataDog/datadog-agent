// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// create a new config to access default config values
var defaultAgentConfig = config.New()

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

		p := testutil.GeneratePayload(10, &testutil.TraceConfig{
			MinSpans: 10,
			Keep:     true,
		}, nil)
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
			payloadsEqual(t, append(p[:2], p[3:]...), v)
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
			assert.Equal(t, "SELECT secret FROM users WHERE id = ?", v.TracerPayloads[0].Chunks[0].Spans[0].Resource)
			for _, s := range v.TracerPayloads[0].Chunks[0].Spans {
				assert.Len(t, s.Service, 100)
				assert.Len(t, s.Name, 100)
			}
		})
	})
}

// payloadsEqual validates that the traces in from are the same as the ones in to.
func payloadsEqual(t *testing.T, from pb.Traces, to *pb.AgentPayload) {
	got := 0
	for _, tracerPayload := range to.TracerPayloads {
		got += len(tracerPayload.Chunks)
	}
	if want := len(from); want != got {
		t.Fatalf("Expected %d traces, got %d", want, got)
	}
	var found int
	for _, t1 := range from {
		for _, tracerPayload := range to.TracerPayloads {
			for _, t2 := range tracerPayload.Chunks {
				if tracesEqual(t1, t2) {
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

// tracesEqual reports whether from and to are equal traces. The latter is allowed
// to contain additional tags.
func tracesEqual(from pb.Trace, to *pb.TraceChunk) bool {
	if want, got := len(from), len(to.Spans); want != got {
		return false
	}
	var found int
	for _, s1 := range from {
		for _, s2 := range to.Spans {
			if spansEqual(s1, s2) {
				found++
				break
			}
		}
	}
	return found == len(from)
}

// spansEqual reports whether s1 and s2 are equal spans. s2 is permitted to have
// tags not present in s1, but not the other way around.
func spansEqual(s1, s2 *pb.Span) bool {
	if s1.Name != s2.Name ||
		s1.Service != s2.Service ||
		// obfuscation changes resource so we leave it out
		// s1.Resource != s2.Resource ||
		s1.TraceID != s2.TraceID ||
		s1.SpanID != s2.SpanID ||
		s1.ParentID != s2.ParentID ||
		s1.Start != s2.Start ||
		s1.Duration != s2.Duration ||
		s1.Error != s2.Error ||
		s1.Type != s2.Type {
		return false
	}
	for k := range s1.Meta {
		if _, ok := s2.Meta[k]; !ok {
			return false
		}
	}
	for k := range s1.Metrics {
		if _, ok := s2.Metrics[k]; !ok {
			return false
		}
	}
	return true
}
