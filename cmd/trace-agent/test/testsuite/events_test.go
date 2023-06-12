// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

func jsonTraceFromPath(path string) (pb.Trace, error) {
	slurp, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t pb.Trace
	if err := json.Unmarshal(slurp, &t); err != nil {
		return t, err
	}
	return t, err
}

func TestAPMEvents(t *testing.T) {
	runner := test.Runner{}
	if err := runner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runner.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	traceWithRates, err := jsonTraceFromPath("./testdata/trace_with_rates.json")
	if err != nil {
		t.Fatal(err)
	}
	traceWithoutRates, err := jsonTraceFromPath("./testdata/trace_without_rates.json")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("off", func(t *testing.T) {
		if err := runner.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer runner.KillAgent()

		if err := runner.Post(pb.Traces{traceWithoutRates}); err != nil {
			t.Fatal(err)
		}

		waitForTrace(t, &runner, func(v *pb.AgentPayload) {
			if n := countEvents(v); n != 0 {
				t.Fatalf("expected no events, got %d", n)
			}
		})
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("DD_APM_ANALYZED_SPANS", "coffee-house|servlet.request=1")
		if err := runner.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer runner.KillAgent()

		if err := runner.Post(pb.Traces{traceWithoutRates}); err != nil {
			t.Fatal(err)
		}

		waitForTrace(t, &runner, func(v *pb.AgentPayload) {
			if n := countEvents(v); n != 1 {
				t.Fatalf("expected 1 event, got %d", n)
			}
		})
	})

	t.Run("client", func(t *testing.T) {
		if err := runner.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer runner.KillAgent()

		if err := runner.Post(pb.Traces{traceWithRates}); err != nil {
			t.Fatal(err)
		}

		waitForTrace(t, &runner, func(v *pb.AgentPayload) {
			if n := countEvents(v); n != 5 {
				t.Fatalf("expected 5 event, got %d", n)
			}
		})
	})
}

func countEvents(p *pb.AgentPayload) int {
	n := 0
	for _, tp := range p.TracerPayloads {
		for _, chunk := range tp.Chunks {
			for _, span := range chunk.Spans {
				if sampler.IsAnalyzedSpan(span) {
					n++
				}
			}
		}
	}
	return n
}
