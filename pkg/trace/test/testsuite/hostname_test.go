package testsuite

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
	"github.com/StackVista/stackstate-agent/pkg/trace/sampler"
	"github.com/StackVista/stackstate-agent/pkg/trace/test"
	"github.com/StackVista/stackstate-agent/pkg/trace/test/testutil"
)

func TestMain(m *testing.M) {
	if _, ok := os.LookupEnv("INTEGRATION"); !ok {
		fmt.Println("--- SKIP: to run tests in this package, set the INTEGRATION environment variable")
		return
	}
	os.Exit(m.Run())
}

func TestHostname(t *testing.T) {
	r := test.Runner{Verbose: true}
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
			payload[0][0].Metrics[sampler.KeySamplingPriority] = 2
			if err := r.Post(payload); err != nil {
				t.Fatal(err)
			}
			waitForTrace(t, r.Out(), func(v pb.TracePayload) {
				if n := len(v.Traces); n != 1 {
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
		os.Setenv("DD_HOSTNAME", "my-env-host")
		defer os.Unsetenv("DD_HOSTNAME")
		testHostname([]byte(`hostname: my-host`), "my-env-host")(t)
	})

	t.Run("auto", func(t *testing.T) {
		if err := r.RunAgent(nil); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		payload := pb.Traces{pb.Trace{testutil.RandomSpan()}}
		payload[0][0].Metrics[sampler.KeySamplingPriority] = 2
		if err := r.Post(payload); err != nil {
			t.Fatal(err)
		}
		waitForTrace(t, r.Out(), func(v pb.TracePayload) {
			if n := len(v.Traces); n != 1 {
				t.Fatalf("expected %d traces, got %d", len(payload), n)
			}
			if v.HostName == "" {
				t.Fatal("hostname detection failed")
			}
		})
	})
}

// waitForTrace waits on the out channel until it times out or receives an pb.TracePayload.
// If the latter happens it will call fn.
func waitForTrace(t *testing.T, out <-chan interface{}, fn func(pb.TracePayload)) {
	waitForTraceTimeout(t, out, 3*time.Second, fn)
}

// waitForTraceTimeout behaves like waitForTrace but allows a customizable wait time.
func waitForTraceTimeout(t *testing.T, out <-chan interface{}, wait time.Duration, fn func(pb.TracePayload)) {
	timeout := time.After(wait)
	for {
		select {
		case p := <-out:
			if v, ok := p.(pb.TracePayload); ok {
				fn(v)
				return
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
}
