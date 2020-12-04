// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package testsuite

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test"
)

func jsonTraceFromPath(path string) (pb.Trace, error) {
	slurp, err := ioutil.ReadFile(path)
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

		waitForTrace(t, &runner, func(v pb.TracePayload) {
			if n := len(v.Transactions); n != 0 {
				t.Fatalf("expected no events, got %d", n)
			}
		})
	})

	t.Run("env", func(t *testing.T) {
		os.Setenv("DD_APM_ANALYZED_SPANS", "coffee-house|servlet.request=1")
		defer os.Unsetenv("DD_APM_ANALYZED_SPANS")
		if err := runner.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer runner.KillAgent()

		if err := runner.Post(pb.Traces{traceWithoutRates}); err != nil {
			t.Fatal(err)
		}

		waitForTrace(t, &runner, func(v pb.TracePayload) {
			if n := len(v.Transactions); n != 1 {
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

		waitForTrace(t, &runner, func(v pb.TracePayload) {
			if n := len(v.Transactions); n != 5 {
				t.Fatalf("expected 5 event, got %d", n)
			}
		})
	})
}
