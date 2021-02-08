package testsuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
)

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
		waitForTrace(t, &r, func(v pb.TracePayload) {
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
			for _, span := range p[i] {
				span.Metrics[sampler.KeySamplingPriority] = -1
			}
		}
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v pb.TracePayload) {
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
		waitForTrace(t, &r, func(v pb.TracePayload) {
			payloadsEqual(t, append(p[:2], p[3:]...), v)
		})
	})

	t.Run("filter_tags", func(t *testing.T) {
		if err := r.RunAgent([]byte("apm_config:\r\n  filter_tags:\r\n require: "["env:prod", db:mysql"]" \r\n"reject": ["outcome:"success"]}]")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		p := testutil.GeneratePayload(5, &testutil.TraceConfig{
			MinSpans: 5,
			Keep:     true,
		}, nil)
		for _, span := range p[2] {
			span.Meta = map[string]string{
				"env": "prod",
				"db":  "mysql",
			}
		}
		if err := r.Post(p); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v pb.TracePayload) {
			payloadsEqual(t, append(p[:2], p[3:]...), v)
		})
	})
}

// payloadsEqual validates that the traces in from are the same as the ones in to.
func payloadsEqual(t *testing.T, from pb.Traces, to pb.TracePayload) {
	if want, got := len(from), len(to.Traces); want != got {
		t.Fatalf("Expected %d traces, got %d", want, got)
	}
	var found int
	for _, t1 := range from {
		for _, t2 := range to.Traces {
			if tracesEqual(t1, t2) {
				found++
				break
			}
		}
	}
	if found != len(from) {
		t.Fatalf("Failed to match traces")
	}
}

// tracesEqual reports whether from and to are equal traces. The latter is allowed
// to contain additional tags.
func tracesEqual(from pb.Trace, to *pb.APITrace) bool {
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
