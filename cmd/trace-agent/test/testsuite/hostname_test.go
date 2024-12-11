// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		log.Println("--- SKIP: to run tests in this package, set the INTEGRATION environment variable")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHostname(t *testing.T) {
	r := test.Runner{}
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	// testHostname returns a test which asserts that for the given agent conf, the
	// expectedHostname is sent to the backend.
	testHostname := func(conf []byte, expectedHostname string) func(*testing.T) {
		return func(t *testing.T) {
			if err := r.RunAgent(conf); err != nil {
				t.Fatal(err)
			}
			defer r.KillAgent()

			payload := pb.Traces{pb.Trace{testutil.RandomSpan()}}
			payload[0][0].Metrics["_sampling_priority_v1"] = 2
			if err := r.Post(payload); err != nil {
				t.Fatal(err)
			}
			waitForTrace(t, &r, func(v *pb.AgentPayload) {
				if n := len(v.TracerPayloads); n != 1 {
					t.Fatalf("expected %d tracer payloads, got %d", 1, n)
				}
				if n := len(v.TracerPayloads[0].Chunks); n != 1 {
					t.Fatalf("expected %d traces, got %d", len(payload), n)
				}
				if v.HostName != expectedHostname {
					t.Fatalf("expected %q, got %q", expectedHostname, v.HostName)
				}
			})
		}
	}

	t.Run("from-config", testHostname([]byte(`hostname: asdq`), "asdq"))

	t.Run("env", func(t *testing.T) {
		t.Setenv("DD_HOSTNAME", "my-env-host")
		testHostname([]byte(`hostname: my-host`), "my-env-host")(t)
	})

	t.Run("auto", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		payload := pb.Traces{pb.Trace{testutil.RandomSpan()}}
		payload[0][0].Metrics["_sampling_priority_v1"] = 2
		if err := r.Post(payload); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, &r, func(v *pb.AgentPayload) {
			if n := len(v.TracerPayloads); n != 1 {
				t.Fatalf("expected %d tracer payloads, got %d", 1, n)
			}
			if n := len(v.TracerPayloads[0].Chunks); n != 1 {
				t.Fatalf("expected %d traces, got %d", len(payload), n)
			}
			if v.HostName == "" {
				t.Fatal("hostname detection failed")
			}
		})
	})
}

// waitForTrace waits on the out channel until it times out or receives an pb.AgentPayload.
// If the latter happens it will call fn.
func waitForTrace(t *testing.T, runner *test.Runner, fn func(*pb.AgentPayload)) {
	waitForTraceTimeout(t, runner, 3*time.Second, fn)
}

// waitForTraceTimeout behaves like waitForTrace but allows a customizable wait time.
func waitForTraceTimeout(t *testing.T, runner *test.Runner, wait time.Duration, fn func(*pb.AgentPayload)) {
	timeout := time.After(wait)
	out := runner.Out()
	for {
		select {
		case p := <-out:
			if v, ok := p.(*pb.AgentPayload); ok {
				fn(v)
				return
			}
		case <-timeout:
			t.Fatalf("timed out, log was:\n%s", runner.AgentLog())
		}
	}
}
