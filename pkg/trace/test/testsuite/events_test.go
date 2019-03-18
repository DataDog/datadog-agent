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

func writeTransactions(t *testing.T, v pb.TracePayload, path string) {
	b, err := json.MarshalIndent(v.Transactions, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(path, b, 0666); err != nil {
		t.Fatal(err)
	}
}

func TestAPMEvents(t *testing.T) {
	runner := test.Runner{Verbose: true}
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
			writeTransactions(t, v, "./off.json")
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
			writeTransactions(t, v, "./env.json")
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
			writeTransactions(t, v, "./client.json")
		})
	})
}
