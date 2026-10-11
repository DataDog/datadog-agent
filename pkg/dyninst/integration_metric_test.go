// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package dyninst_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// fakeMetricProbeStatsd records the metrics reported by metric probes.
type fakeMetricProbeStatsd struct {
	ddgostatsd.NoOpClient
	mu     sync.Mutex
	counts map[string]int64
	gauges map[string]float64
	tags   map[string][]string
}

func newFakeMetricProbeStatsd() *fakeMetricProbeStatsd {
	return &fakeMetricProbeStatsd{
		counts: make(map[string]int64),
		gauges: make(map[string]float64),
		tags:   make(map[string][]string),
	}
}

func (f *fakeMetricProbeStatsd) Count(name string, value int64, tags []string, _ float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[name] += value
	f.tags[name] = tags
	return nil
}

func (f *fakeMetricProbeStatsd) Gauge(name string, value float64, tags []string, _ float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gauges[name] = value
	f.tags[name] = tags
	return nil
}

func (f *fakeMetricProbeStatsd) snapshot() (map[string]int64, map[string]float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := make(map[string]int64, len(f.counts))
	for k, v := range f.counts {
		counts[k] = v
	}
	gauges := make(map[string]float64, len(f.gauges))
	for k, v := range f.gauges {
		gauges[k] = v
	}
	return counts, gauges
}

// metricPrefix is the prefix of the names of the metrics of metric probes.
const metricPrefix = "dynamic.instrumentation.metric.probe."

// metricProbeDefinition returns a metric probe whose metric is named after the
// probe. extra is appended to the probe's JSON object.
func metricProbeDefinition(t *testing.T, id, kind, where, extra string) ir.ProbeDefinition {
	js := fmt.Sprintf(
		`{"id": %q, "type": "METRIC_PROBE", "version": 1, "kind": %q, "metricName": %q, "where": %s%s}`,
		id, kind, id, where, extra,
	)
	p, err := rcjson.UnmarshalProbe([]byte(js))
	require.NoError(t, err, js)
	return p
}

// onFunction returns the where clause of a probe on a function.
func onFunction(name string) string { return fmt.Sprintf(`{"methodName": %q}`, name) }

// metricValue returns the JSON field of a metric probe's value, to be appended
// to the probe's JSON object.
func metricValue(dsl, js string) string {
	return fmt.Sprintf(`, "value": {"dsl": %q, "json": %s}`, dsl, js)
}

// startMetricProbesModule starts a dyninst module that reports metric probes
// to a fake DogStatsD client. It returns the fake agent that the module uploads
// statuses and logs to, the fake client, and a function that sends process
// updates to the module.
func startMetricProbesModule(
	t *testing.T, tempDir string,
) (*fakeAgent, *fakeMetricProbeStatsd, func(process.ProcessesUpdate)) {
	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)
	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	modCfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	modCfg.LogUploaderURL = testServer.getLogsURL()
	modCfg.DiagsUploaderURL = testServer.getDiagsURL()
	var sendUpdate fakeProcessSubscriber
	modCfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close() // prevent start from doing anything
		return &sendUpdate
	}
	modCfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	modCfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &backoff.ExpBackoffPolicy{MaxBackoffTime: time.Millisecond.Seconds()},
	}
	modCfg.ActuatorConfig.RecompilationRateLimit = -1 // disable recompilation
	statsd := newFakeMetricProbeStatsd()
	m, err := module.NewModule(modCfg, nil, module.WithMetricProbeStatsd(statsd))
	require.NoError(t, err)
	t.Cleanup(m.Close)
	return testServer, statsd, func(u process.ProcessesUpdate) { sendUpdate(u) }
}

// metricProbeStatuses returns the most advanced status reported for each
// probe. It fails the test if a probe reported an error.
func metricProbeStatuses(t *testing.T, testServer *fakeAgent) map[string]uploader.Status {
	statusRank := map[uploader.Status]int{
		uploader.StatusReceived:  1,
		uploader.StatusInstalled: 2,
		uploader.StatusEmitting:  3,
	}
	ret := make(map[string]uploader.Status)
	for _, d := range testServer.getDiags() {
		msg := d.diagnosticMessage.Debugger
		if msg.Status == uploader.StatusError {
			t.Fatalf("probe %s failed: %s", msg.ProbeID, msg.DiagnosticException.Message)
		}
		if statusRank[msg.Status] > statusRank[ret[msg.ProbeID]] {
			ret[msg.ProbeID] = msg.Status
		}
	}
	return ret
}

// TestMetricProbes puts metric probes on the functions of the simple test
// program, runs it, and checks the reported metrics.
func TestMetricProbes(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	for _, cfg := range testprogs.MustGetCommonConfigs(t) {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
			}
			testMetricProbes(t, cfg)
		})
	}
}

func testMetricProbes(t *testing.T, cfg testprogs.Config) {
	// The simple program calls each of these functions twice. See main.go.
	probes := []ir.ProbeDefinition{
		metricProbeDefinition(t, "calls", "COUNT", onFunction("main.condInt64"), ""),
		metricProbeDefinition(t, "sum", "COUNT", onFunction("main.condInt64"),
			metricValue("x", `{"ref": "x"}`)),
		metricProbeDefinition(t, "sum_when", "COUNT", onFunction("main.condInt64"),
			metricValue("x", `{"ref": "x"}`)+
				`, "when": {"dsl": "tag == \"match\"", "json": {"eq": [{"ref": "tag"}, "match"]}}`),
		metricProbeDefinition(t, "unsigned_sum", "COUNT", onFunction("main.condUint64"),
			metricValue("x", `{"ref": "x"}`)),
		metricProbeDefinition(t, "bools", "COUNT", onFunction("main.condBool"),
			metricValue("x", `{"ref": "x"}`)),
		metricProbeDefinition(t, "gauge", "GAUGE", onFunction("main.condPtrStructArg"),
			metricValue("x.F64", `{"getmember": [{"ref": "x"}, "F64"]}`)),
		// The second call passes a nil pointer.
		metricProbeDefinition(t, "nil_field", "COUNT", onFunction("main.condNilPtrStruct"),
			metricValue("x.I32", `{"getmember": [{"ref": "x"}, "I32"]}`)),
	}
	wantCounts := map[string]int64{
		metricPrefix + "calls":        2,
		metricPrefix + "sum":          42 + 7,
		metricPrefix + "sum_when":     42,
		metricPrefix + "unsigned_sum": 42 + 7,
		metricPrefix + "bools":        1,
		metricPrefix + "nil_field":    300,
	}
	wantGauges := map[string]float64{
		metricPrefix + "gauge": 0.2,
	}
	// go1.23's location lists for main.condLine are bad (the probe files skip
	// line probes on it too).
	if !strings.HasPrefix(cfg.GOTOOLCHAIN, "go1.23") {
		probes = append(probes, metricProbeDefinition(t, "line", "COUNT",
			`{"methodName": "main.condLine", "sourceFile": "main.go", "lines": ["754"]}`,
			metricValue("n", `{"ref": "n"}`)))
		wantCounts[metricPrefix+"line"] = 7
	}

	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-metric-test")
	defer cleanup()
	testServer, statsd, sendUpdate := startMetricProbesModule(t, tempDir)

	bin := testprogs.MustGetBinary(t, "simple", cfg)
	proc, stdin := dyninsttest.StartProcess(context.Background(), t, tempDir, bin)
	defer func() {
		_ = proc.Process.Kill()
		_ = proc.Wait()
	}()
	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(proc.Process.Pid))
	require.NoError(t, err)
	sendUpdate(process.ProcessesUpdate{Updates: []process.Config{{
		Info: process.Info{
			ProcessID:   process.ID{PID: int32(proc.Process.Pid)},
			Executable:  exe,
			Service:     "simple",
			Environment: "test",
		},
		RuntimeID: "metric-test",
		Probes:    probes,
	}}})

	statuses := func() map[string]uploader.Status { return metricProbeStatuses(t, testServer) }
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		got := statuses()
		for _, p := range probes {
			assert.Contains(c, []uploader.Status{uploader.StatusInstalled, uploader.StatusEmitting},
				got[p.GetID()], "probe %s", p.GetID())
		}
	}, 3*time.Minute, 100*time.Millisecond, "probes should be installed")

	// Run the program's calls and let it exit. Exiting unloads the program,
	// which reports the metric probes one last time.
	_, err = stdin.Write([]byte("\n"))
	require.NoError(t, err)
	require.NoError(t, proc.Wait())
	sendUpdate(process.ProcessesUpdate{
		Removals: []process.ID{{PID: int32(proc.Process.Pid)}},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		counts, gauges := statsd.snapshot()
		assert.Equal(c, wantCounts, counts)
		assert.InDeltaMapValues(c, wantGauges, gauges, 1e-9)
	}, 30*time.Second, 100*time.Millisecond, "metrics should be reported")

	statsd.mu.Lock()
	assert.Equal(t, []string{"debugger.probeid:calls", "service:simple", "env:test"},
		statsd.tags[metricPrefix+"calls"])
	statsd.mu.Unlock()

	// All the probes that were hit are emitting.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		got := statuses()
		for _, p := range probes {
			assert.Equal(c, uploader.StatusEmitting, got[p.GetID()], "probe %s", p.GetID())
		}
	}, 30*time.Second, 100*time.Millisecond, "probes should be emitting")

	// The nil pointer dereference was reported as an evaluation error.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var found bool
		for _, l := range testServer.getLogs() {
			if l.id != "nil_field" {
				continue
			}
			var event struct {
				Debugger struct {
					Snapshot struct {
						EvaluationErrors []struct {
							Expr    string `json:"expr"`
							Message string `json:"message"`
						} `json:"evaluationErrors"`
					} `json:"snapshot"`
				} `json:"debugger"`
			}
			if !assert.NoError(c, json.Unmarshal(l.body, &event)) {
				return
			}
			errs := event.Debugger.Snapshot.EvaluationErrors
			if assert.Len(c, errs, 1) {
				assert.Equal(c, "x.I32", errs[0].Expr)
				assert.Equal(c, "nil pointer dereference", errs[0].Message)
			}
			found = true
		}
		assert.True(c, found, "no evaluation error for probe nil_field")
	}, 30*time.Second, 100*time.Millisecond)
	// Metric probes don't produce snapshots otherwise.
	for _, l := range testServer.getLogs() {
		assert.Equal(t, "nil_field", l.id, "unexpected log: %s", l.body)
	}
}
