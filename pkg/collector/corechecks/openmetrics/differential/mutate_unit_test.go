// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"bytes"
	"testing"
)

const miniPayload = `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="get",code="200"} 42
http_requests_total{method="post",code="500"} 7
# HELP request_duration_seconds Request duration in seconds
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 1
request_duration_seconds_bucket{le="1.0"} 5
request_duration_seconds_bucket{le="+Inf"} 10
request_duration_seconds_count 10
request_duration_seconds_sum 3.14
`

// TestMutatorDeterministic asserts the contract that lets fuzz seeds reproduce
// failures: same seed + same input -> same output, byte-for-byte.
func TestMutatorDeterministic(t *testing.T) {
	input := []byte(miniPayload)
	a := NewMutator(42).Mutate(input, 5)
	b := NewMutator(42).Mutate(input, 5)
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic mutation:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

// TestMutatorActuallyMutates asserts the mutator isn't trivially a no-op for a
// representative payload. Some individual ops are no-ops on shapeless inputs,
// but with five mutations over a payload that has counters + histograms +
// labels, at least one should land.
func TestMutatorActuallyMutates(t *testing.T) {
	input := []byte(miniPayload)
	changed := 0
	for seed := int64(1); seed <= 20; seed++ {
		out := NewMutator(seed).Mutate(input, 5)
		if !bytes.Equal(input, out) {
			changed++
		}
	}
	if changed < 18 { // allow a couple unlucky no-op runs
		t.Fatalf("mutator changed only %d/20 inputs \u2014 op weights or applicability broken?", changed)
	}
}

// TestParseSample is a smoke test of the lightweight sample-line parser; if
// this breaks, label-aware mutations silently downgrade to no-ops.
func TestParseSample(t *testing.T) {
	cases := []struct {
		in            string
		wantName      string
		wantHasLabels bool
		wantValue     string
	}{
		{`metric 42`, "metric", false, "42"},
		{`metric{} 42`, "metric", true, "42"},
		{`metric{a="b"} 42`, "metric", true, "42"},
		{`metric{a="b,c",d="e"} 42`, "metric", true, "42"},
		{`metric{a="with \"quote\""} 42`, "metric", true, "42"},
		{`metric{a="b"} 42 1234567890`, "metric", true, "42"},
	}
	for _, tc := range cases {
		p, ok := parseSample(tc.in)
		if !ok {
			t.Errorf("parseSample(%q) failed", tc.in)
			continue
		}
		if p.name != tc.wantName {
			t.Errorf("%q: name = %q, want %q", tc.in, p.name, tc.wantName)
		}
		if (p.labels != "") != tc.wantHasLabels {
			t.Errorf("%q: labels = %q, want hasLabels=%v", tc.in, p.labels, tc.wantHasLabels)
		}
		if p.value != tc.wantValue {
			t.Errorf("%q: value = %q, want %q", tc.in, p.value, tc.wantValue)
		}
	}
}
