// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package preflightmodeimpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// These tests drive the real os/exec, capture, signal and DogStatsD paths against a
// stand-in for ADP: this test binary, re-executed with fakeEnabledVar set (see TestMain).
// A real subprocess is used rather than a mocked command builder so that the parts most
// likely to break in production — pipe handling, stop-signal delivery, unix datagram
// socket delivery — are actually exercised.
//
// Windows is excluded: the named pipe listener and the kill-based stop have no equivalent
// here. Those are covered by the E2E.
const (
	// Neither variable may be DD_-prefixed: sanitizedEnv strips the whole DD_ namespace
	// from the child's environment, so a DD_-prefixed name would never reach the
	// stand-in.
	fakeEnabledVar = "DDTEST_FAKE_DATA_PLANE"
	fakeModeVar    = "DDTEST_FAKE_DATA_PLANE_MODE"

	modeNormal           = "normal"
	modeErrors           = "errors"
	modeInvalidAPIKey    = "invalid_api_key"
	modeExitEarly        = "exit_early"
	modeCrash            = "crash"
	modePanic            = "panic"
	modeIgnoreStopSignal = "ignore_stop_signal"
	modeNoListener       = "no_listener"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnabledVar) == "1" {
		os.Exit(runFakeDataPlane())
	}
	os.Exit(m.Run())
}

// adpLog writes one log record in ADP's JSON format. Preflight mode forces log_format_json, so
// the stand-in must emit the same shape as the real binary or the tests would be exercising
// a format that never occurs.
func adpLog(w io.Writer, level, target, message string) {
	rec, err := json.Marshal(map[string]any{
		"timestamp":   "2026-07-27T12:00:00.000000Z",
		"level":       level,
		"message":     message,
		"target":      target,
		"filename":    "bin/agent-data-plane/src/main.rs",
		"line_number": 1,
	})
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(w, "%s\n", rec)
}

// runFakeDataPlane stands in for the agent-data-plane binary. It is invoked with exactly
// the arguments the component passes to the real one, reads the generated config the same
// way, emits the same JSON log format, and shuts down on the same signal.
func runFakeDataPlane() int {
	cfgPath := ""
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
		}
	}
	if cfgPath == "" {
		adpLog(os.Stderr, "ERROR", "fake_adp", "no --config argument")
		return 3
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		adpLog(os.Stderr, "ERROR", "fake_adp", fmt.Sprintf("could not read config: %v", err))
		return 3
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		adpLog(os.Stderr, "ERROR", "fake_adp", fmt.Sprintf("could not parse config: %v", err))
		return 3
	}
	socket, _ := cfg["dogstatsd_socket"].(string)

	// Report the contract the real binary depends on, so the tests can assert on it rather
	// than trusting the stand-in to be invoked correctly.
	if !slices.Contains(os.Args, "run") {
		adpLog(os.Stderr, "ERROR", "fake_adp", "missing the 'run' subcommand")
		return 3
	}
	for _, kv := range os.Environ() {
		// Case-insensitive, matching sanitizedEnv: on Windows a lowercase dd_dogstatsd_port
		// still reaches this process as DD_DOGSTATSD_PORT.
		if len(kv) >= 3 && strings.EqualFold(kv[:3], "DD_") {
			adpLog(os.Stderr, "ERROR", "fake_adp",
				"inherited a DD_ environment variable: "+strings.SplitN(kv, "=", 2)[0])
			return 3
		}
	}
	// The footprint overrides reach the real binary the same way, so report the one the tests can
	// observe. Read rather than scanned, so an inherited duplicate that childEnv failed to remove
	// shows up here as whichever value the exec actually resolved to.
	if got := os.Getenv("TOKIO_WORKER_THREADS"); got != "2" {
		adpLog(os.Stderr, "ERROR", "fake_adp", "TOKIO_WORKER_THREADS is "+got+", expected 2")
		return 3
	}

	mode := os.Getenv(fakeModeVar)
	if mode == "" {
		mode = modeNormal
	}
	adpLog(os.Stdout, "INFO", "agent_data_plane::cli::run",
		fmt.Sprintf("Agent Data Plane starting... (mode %s)", mode))
	// The real binary always emits this because preflight mode sets standalone_mode. Every
	// mode emits it so the expected-warning filter is exercised on every run.
	adpLog(os.Stderr, "WARN", "agent_data_plane::internal::env",
		"Running in standalone mode. Origin detection/enrichment and other features dependent upon the Datadog Agent will not be available.")

	switch mode {
	case modeExitEarly:
		adpLog(os.Stdout, "INFO", "agent_data_plane::cli::run", "nothing to do, exiting")
		return 0
	case modeCrash:
		adpLog(os.Stderr, "ERROR", "agent_data_plane", "unrecoverable startup failure")
		return 2
	case modePanic:
		// A Rust panic bypasses the logger and goes straight to stderr as plain text.
		fmt.Fprintln(os.Stderr, "thread 'main' panicked at bin/agent-data-plane/src/main.rs:42:5:")
		fmt.Fprintln(os.Stderr, "note: run with `RUST_BACKTRACE=1` environment variable to display a backtrace")
		return 101
	}

	if mode != modeNoListener {
		if socket == "" {
			adpLog(os.Stderr, "ERROR", "fake_adp", "no dogstatsd_socket in the generated config")
			return 3
		}
		conn, err := net.ListenPacket("unixgram", socket)
		if err != nil {
			adpLog(os.Stderr, "ERROR", "saluki_components::sources::dogstatsd",
				fmt.Sprintf("Failed to create unixgram listener on unixgram://%s: %v", socket, err))
			return 3
		}
		defer conn.Close()
		adpLog(os.Stdout, "INFO", "saluki_components::sources::dogstatsd", "DogStatsD listener started.")
		go func() {
			buf := make([]byte, 8192)
			for {
				n, _, err := conn.ReadFrom(buf)
				if err != nil {
					return
				}
				// Echoed so a test can assert the probe actually arrived, and so it
				// travels the capture path like any other output line.
				adpLog(os.Stdout, "INFO", "fake_adp", fmt.Sprintf("received %q", string(buf[:n])))
			}
		}()
	}

	if mode == modeErrors {
		// Two records that differ only in a retry count, so they must collapse into one
		// signature, plus a multi-line anyhow chain like the real binary emits.
		adpLog(os.Stderr, "ERROR", "saluki_io::net", "connection refused (attempt 1)")
		adpLog(os.Stderr, "ERROR", "saluki_io::net", "connection refused (attempt 2)")
		adpLog(os.Stderr, "ERROR", "agent_data_plane",
			"Failed to create internal supervisor.\n\nCaused by:\n    No such file or directory (os error 2)")
	}

	if mode == modeInvalidAPIKey {
		// The real binary reports a rejected key at WARN, from this target.
		adpLog(os.Stderr, "WARN", "saluki_components::common::datadog::validation",
			"Datadog API key is invalid.")
	}

	// The real binary only handles SIGINT; see the stopSignal comment in terminate_nix.go.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)

	if mode == modeIgnoreStopSignal {
		// Deliberately never returns: the component must escalate to a kill.
		time.Sleep(2 * time.Minute)
		return 0
	}

	<-sigs
	adpLog(os.Stdout, "INFO", "agent_data_plane::cli::run", "Agent Data Plane shut down successfully.")
	return 0
}

// testLifecycle is a minimal compdef.Lifecycle that records and replays hooks.
type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func (l *testLifecycle) start(t *testing.T) {
	t.Helper()
	for _, h := range l.hooks {
		if h.OnStart != nil {
			require.NoError(t, h.OnStart(t.Context()))
		}
	}
}

func (l *testLifecycle) stop(t *testing.T) {
	t.Helper()
	require.NoError(t, l.stopErr(t.Context()))
}

// stopErr runs the OnStop hooks and returns any error instead of failing the test, so that
// callers can invoke it from a goroutine. testing forbids t.FailNow (which require uses)
// off the test goroutine — it can hang or misreport.
func (l *testLifecycle) stopErr(ctx context.Context) error {
	for _, h := range l.hooks {
		if h.OnStop != nil {
			if err := h.OnStop(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// shortTempDir returns a temporary directory with a short path, for use as run_path.
// t.TempDir() is unusable here: on macOS it lives under /var/folders/... and the resulting
// socket path overruns the ~104 byte sockaddr_un limit (see listener.validate).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ddadp")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// setPreflightModeDuration shortens (or lengthens) the run window for one test. preflightModeDuration
// is read when the timer is armed, so this must be called before the lifecycle starts.
func setPreflightModeDuration(t *testing.T, d time.Duration) {
	t.Helper()
	original := preflightModeDuration
	preflightModeDuration = d
	t.Cleanup(func() { preflightModeDuration = original })
}

// useFakeDataPlane points the component at this test binary and selects a behaviour.
func useFakeDataPlane(t *testing.T, mode string) {
	t.Helper()
	t.Setenv(fakeEnabledVar, "1")
	t.Setenv(fakeModeVar, mode)

	self, err := os.Executable()
	require.NoError(t, err)
	old := resolveDataPlanePath
	resolveDataPlanePath = func() string { return self }
	t.Cleanup(func() { resolveDataPlanePath = old })
}

type harness struct {
	comp *preflightModeComponent
	lc   *testLifecycle
	tlm  telemetry.Mock
	cfg  pkgconfigmodel.Config
}

// newHarness wires the component against the fake ADP with short timings.
func newHarness(t *testing.T, mode string, tweak func(pkgconfigmodel.Config)) *harness {
	t.Helper()
	useFakeDataPlane(t, mode)
	// The real window is 90s; every lifecycle test would pay that in wall clock.
	setPreflightModeDuration(t, 3*time.Second)

	cfg := configmock.New(t)
	cfg.Set("run_path", shortTempDir(t), pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("api_key", "0123456789abcdef0123456789abcdef", pkgconfigmodel.SourceFile)
	cfg.Set(DataPlaneStopTimeout, 2, pkgconfigmodel.SourceAgentRuntime)
	if tweak != nil {
		tweak(cfg)
	}

	tlm := telemetrymock.New(t)
	lc := &testLifecycle{}
	p := NewComponent(Requires{Lc: lc, Config: cfg, Log: logmock.New(t), Telemetry: tlm})

	comp, ok := p.Comp.(*preflightModeComponent)
	require.True(t, ok)
	return &harness{comp: comp, lc: lc, tlm: tlm, cfg: cfg}
}

// runToCompletion starts the component and waits for the pre-flight to finish.
func (h *harness) runToCompletion(t *testing.T) {
	t.Helper()
	require.NotNil(t, h.comp.done, "the component is inert; it will never run")
	h.lc.start(t)
	select {
	case <-h.comp.done:
	case <-time.After(90 * time.Second):
		t.Fatal("preflight mode did not finish")
	}
	h.lc.stop(t)
}

// captured returns the ADP log records retained so far.
func (h *harness) captured() []logRecord {
	if h.comp.out == nil {
		return nil
	}
	records, _ := h.comp.out.snapshot()
	return records
}

// capturedContains reports whether any retained record's message contains sub. The retained
// form is a scrubbed signature, so only substrings that survive scrubbing can be matched —
// digits and paths in particular are collapsed.
func (h *harness) capturedContains(sub string) bool {
	for _, rec := range h.captured() {
		if strings.Contains(rec.Signature, sub) {
			return true
		}
	}
	return false
}

// findingCount returns how many times a finding was reported.
//
// A lookup error means the counter was never registered at all — if that were swallowed and
// reported as 0, every assert.Zero on a finding would be unfalsifiable, and renaming the
// metric would break nothing.
func (h *harness) findingCount(t *testing.T, f finding) float64 {
	t.Helper()
	metrics, err := h.tlm.GetCountMetric(telemetrySubsystem, metricFinding)
	if err != nil {
		// Prometheus only materializes a counter once it has been incremented with some
		// label set, so "not found" is legitimate when a run produced no findings at all.
		// Distinguish that from the metric not existing by checking the result counter,
		// which every completed run increments exactly once.
		_, resultErr := h.tlm.GetCountMetric(telemetrySubsystem, metricResult)
		require.NoErrorf(t, resultErr,
			"neither %s.%s nor %s.%s was registered; the reporter did not run",
			telemetrySubsystem, metricFinding, telemetrySubsystem, metricResult)
		return 0
	}
	for _, m := range metrics {
		if m.Tags()[labelFinding] == string(f) {
			return m.Value()
		}
	}
	return 0
}

// result returns the single reported result label.
func (h *harness) result(t *testing.T) string {
	t.Helper()
	metrics, err := h.tlm.GetCountMetric(telemetrySubsystem, metricResult)
	require.NoError(t, err)
	require.Len(t, metrics, 1, "exactly one result must be reported per run")
	return metrics[0].Tags()[labelResult]
}

func TestPreflightModeCleanRun(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	assert.Equal(t, resultClean, h.result(t))
	for _, f := range allFindings {
		assert.Zerof(t, h.findingCount(t, f), "unexpected finding %s; captured: %v", f, h.captured())
	}

	durations, err := h.tlm.GetGaugeMetric(telemetrySubsystem, metricDuration)
	require.NoError(t, err)
	require.Len(t, durations, 1)
	assert.Greater(t, durations[0].Value(), 0.0)
}

// TestPreflightModeProbeReachesDataPlane is the point of the probe: a DogStatsD sample must
// actually traverse the socket into the data plane process.
func TestPreflightModeProbeReachesDataPlane(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	records := h.captured()
	var received string
	for _, rec := range records {
		if strings.Contains(rec.Signature, probeMetricName) {
			received = rec.Signature
			break
		}
	}
	require.NotEmptyf(t, received, "the probe metric never arrived at the data plane; captured: %v", records)
	assert.Contains(t, received, "preflight_mode:true")
	assert.Zero(t, h.findingCount(t, findingProbeFailed))
}

func TestPreflightModeErrorsInLog(t *testing.T) {
	h := newHarness(t, modeErrors, nil)
	h.runToCompletion(t)

	assert.Equal(t, string(findingErrorsInLog), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingErrorsInLog))
	assert.Zero(t, h.findingCount(t, findingProbeFailed))
	assert.Zero(t, h.findingCount(t, findingWarningsInLog),
		"the standalone-mode warning is provoked by preflight mode itself")
}

// TestPreflightModeInvalidAPIKey is the end-to-end version of the case that motivated
// warnings_in_log: ADP reports a rejected API key at WARN, so a scan that only looked at
// ERROR would report a completely clean run on a host whose key does not work.
func TestPreflightModeInvalidAPIKey(t *testing.T) {
	h := newHarness(t, modeInvalidAPIKey, nil)
	h.runToCompletion(t)

	assert.Equal(t, string(findingWarningsInLog), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingWarningsInLog))
	assert.Zero(t, h.findingCount(t, findingErrorsInLog), "ADP reports this at WARN, not ERROR")
}

func TestPreflightModeExitsEarly(t *testing.T) {
	h := newHarness(t, modeExitEarly, nil)
	h.runToCompletion(t)

	// The process dying is the root cause, so it is the reported result even though the probe
	// also could not be delivered.
	assert.Equal(t, string(findingExitedEarly), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingExitedEarly))
	assert.Zero(t, h.findingCount(t, findingProbeFailed), "the probe failure is only a consequence")
}

func TestPreflightModeCrashes(t *testing.T) {
	h := newHarness(t, modeCrash, nil)
	h.runToCompletion(t)

	assert.Equal(t, string(findingExitedEarly), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingErrorsInLog), "the crash logged an ERROR line")
}

func TestPreflightModeStopTimeout(t *testing.T) {
	h := newHarness(t, modeIgnoreStopSignal, nil)
	h.runToCompletion(t)

	assert.Equal(t, 1.0, h.findingCount(t, findingStopTimeout))
}

// TestPreflightModePanicIsReported covers output that bypassed ADP's logger. A Rust panic goes
// straight to stderr as plain text, so it would be invisible to a scanner that only
// understood JSON records.
func TestPreflightModePanicIsReported(t *testing.T) {
	h := newHarness(t, modePanic, nil)
	h.runToCompletion(t)

	assert.Equal(t, 1.0, h.findingCount(t, findingErrorsInLog),
		"a panic bypasses the JSON logger, so it must still be reported as an error")
}

// TestPreflightModeStopsGracefully pins the stop signal. The real binary only handles SIGINT, so
// sending SIGTERM would kill it outright and skip the graceful shutdown the pre-flight exists
// to exercise.
func TestPreflightModeStopsGracefully(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	assert.Equal(t, resultClean, h.result(t))
	assert.Zero(t, h.findingCount(t, findingStopTimeout),
		"the stop signal must be one the data plane actually handles")

	// The stand-in only logs this after handling the stop signal, so its presence proves
	// the graceful path ran rather than the process being killed outright. It is an INFO
	// record, which is why the capture retains records below a warning as context.
	assert.True(t, h.capturedContains("shut down successfully"),
		"the data plane did not shut down gracefully; captured: %v", h.captured())
}

func TestPreflightModeNoListener(t *testing.T) {
	h := newHarness(t, modeNoListener, nil)
	h.runToCompletion(t)

	assert.Equal(t, string(findingProbeFailed), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingProbeFailed))
}

// TestPreflightModeCleansUpWorkDir matters because the generated config holds a resolved
// api_key: it must not outlive the run.
func TestPreflightModeCleansUpWorkDir(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	workDir := filepath.Join(h.cfg.GetString("run_path"), workDirName)
	h.runToCompletion(t)

	_, err := os.Stat(workDir)
	assert.Truef(t, os.IsNotExist(err), "%s survived the run (err=%v)", workDir, err)
}

// TestPreflightModeStopDuringRun covers an Agent shutdown landing in the middle of the preflight
// window: the pre-flight must unwind rather than leave an orphaned process behind.
func TestPreflightModeStopDuringRun(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	setPreflightModeDuration(t, 120*time.Second)

	h.lc.start(t)
	// Wait until the process is actually up before pulling the rug out.
	require.Eventually(t, func() bool {
		return len(h.captured()) > 0
	}, 30*time.Second, 100*time.Millisecond, "the data plane never produced output")

	stopped := make(chan error, 1)
	go func() { stopped <- h.lc.stopErr(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(maxShutdownGrace + 10*time.Second):
		t.Fatal("OnStop did not return")
	}

	select {
	case <-h.comp.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the run did not unwind after OnStop")
	}

	// An Agent restart inside the window says nothing about the host. Before this was
	// handled, the shutdown path fell through to normal post-processing and shipped
	// no_listener (the probe legitimately never found a listener, because we killed ADP
	// mid-startup) — a permanent floor of false positives under the primary signal, since
	// restarts inside a 60s window are common at fleet scale.
	assert.Equal(t, string(findingInterrupted), h.result(t))
	assert.Equal(t, 1.0, h.findingCount(t, findingInterrupted))
	assert.Zero(t, h.findingCount(t, findingProbeFailed),
		"the probe could not run because we stopped ADP, which is not a host problem")
	assert.Zero(t, h.findingCount(t, findingProbeFailed))
	assert.Zero(t, h.findingCount(t, findingExitedEarly))
}

// TestPreflightModeInterruptedStillReportsRealErrors is the other half: an interrupted run must
// still surface errors ADP actually logged, because those are real regardless of why the run
// ended. Only the findings that are artefacts of us stopping it are suppressed.
func TestPreflightModeInterruptedStillReportsRealErrors(t *testing.T) {
	h := newHarness(t, modeErrors, nil)
	setPreflightModeDuration(t, 120*time.Second)

	h.lc.start(t)
	require.Eventually(t, func() bool {
		return h.capturedContains("connection refused")
	}, 30*time.Second, 100*time.Millisecond, "the data plane never logged its errors")

	stopped := make(chan error, 1)
	go func() { stopped <- h.lc.stopErr(context.Background()) }()
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(maxShutdownGrace + 10*time.Second):
		t.Fatal("OnStop did not return")
	}
	<-h.comp.done

	assert.Equal(t, string(findingInterrupted), h.result(t), "interrupted is recorded first")
	assert.Equal(t, 1.0, h.findingCount(t, findingErrorsInLog),
		"errors ADP actually logged are real even if the run was cut short")
}

func TestPreflightModeInertWhenDataPlaneExplicitlyConfigured(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("data_plane.enabled=%t", enabled), func(t *testing.T) {
			h := newHarness(t, modeNormal, func(cfg pkgconfigmodel.Config) {
				cfg.Set(DataPlaneEnabled, enabled, pkgconfigmodel.SourceFile)
			})
			assert.Nil(t, h.comp.done, "the component should be inert")
			assert.Empty(t, h.lc.hooks, "an inert component must not register lifecycle hooks")
		})
	}
}

// TestPreflightModeStripsDDEnvFromChild is the end-to-end half of sanitizedEnv, which the unit
// test alone cannot provide.
//
// ADP layers environment variables over its config file, so an inherited DD_DOGSTATSD_PORT
// would make the preflight process bind the real DogStatsD endpoint and take traffic away from
// the Core Agent. The stand-in fails the run if it sees any DD_ variable, so deleting the
// sanitizedEnv call in run() now breaks this test — previously it broke nothing.
func TestPreflightModeStripsDDEnvFromChild(t *testing.T) {
	t.Setenv("DD_DOGSTATSD_PORT", "8125")
	t.Setenv("DD_DOGSTATSD_SOCKET", "/var/run/datadog/dsd.socket")
	t.Setenv("DD_API_KEY", "0123456789abcdef0123456789abcdef")
	// Windows environment lookups are case-insensitive, so these reach ADP as the real thing.
	t.Setenv("dd_dogstatsd_port", "8125")
	t.Setenv("Dd_Dogstatsd_Socket", "/var/run/datadog/dsd.socket")

	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	assert.False(t, h.capturedContains("inherited a DD_ environment variable"),
		"the child saw a DD_ variable that sanitizedEnv should have stripped")
	assert.Equal(t, resultClean, h.result(t))
}

// TestPreflightModeCapsChildWorkerThreads is the end-to-end half of childEnv.
//
// Tokio reads TOKIO_WORKER_THREADS itself, so this is the only way the pre-flight can keep ADP's
// runtime — and therefore its idle footprint — from scaling with the host's core count. The
// inherited value is deliberately larger: it proves childEnv replaces rather than shadows, which a
// unit test on the returned slice cannot show, because whether a duplicate wins is up to the exec.
func TestPreflightModeCapsChildWorkerThreads(t *testing.T) {
	t.Setenv("TOKIO_WORKER_THREADS", "64")

	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	assert.False(t, h.capturedContains("TOKIO_WORKER_THREADS is"),
		"the child ran with a worker thread count other than the one childEnv sets")
	assert.Equal(t, resultClean, h.result(t))
}

// TestPreflightModePassesRunSubcommand pins the other half of the invocation contract: the real
// binary requires "--config <path> run", and dropping the subcommand would fail only at
// runtime on a customer host.
func TestPreflightModePassesRunSubcommand(t *testing.T) {
	h := newHarness(t, modeNormal, nil)
	h.runToCompletion(t)

	assert.False(t, h.capturedContains("missing the 'run' subcommand"))
	assert.Equal(t, resultClean, h.result(t))
}

// TestPreflightModeInertOnNonDefaultFlavor covers the flavor gate, which is one of the four
// documented run conditions and previously had no coverage at all — the package's tests all
// run as the default flavor, so the branch was never taken.
//
// flavor.IotAgent is deliberately not driven through SetFlavor here: that writes
// iot_host: true into the *global* config and neither SetFlavor nor flavor.SetTestFlavor
// undoes it, so GetFlavor keeps reporting IoT for every subsequent test in the package and
// silently makes them all inert. The IoT case is covered below through iot_host itself,
// which configmock restores on cleanup.
func TestPreflightModeInertOnNonDefaultFlavor(t *testing.T) {
	for _, f := range []string{flavor.HerokuAgent, flavor.ClusterAgent, flavor.Dogstatsd} {
		t.Run(f, func(t *testing.T) {
			flavor.SetTestFlavor(t, f)

			h := newHarness(t, modeNormal, nil)
			assert.Nil(t, h.comp.done, "preflight mode must not run on the %s flavor", f)
			assert.Empty(t, h.lc.hooks)
		})
	}
}

// TestPreflightModeInertOnIotHost covers the IoT flavor via the setting GetFlavor actually keys
// off, so the global flavor variable is never touched.
func TestPreflightModeInertOnIotHost(t *testing.T) {
	h := newHarness(t, modeNormal, func(cfg pkgconfigmodel.Config) {
		cfg.Set("iot_host", true, pkgconfigmodel.SourceAgentRuntime)
	})

	require.Equal(t, flavor.IotAgent, flavor.GetFlavor(), "iot_host should make GetFlavor report IoT")
	assert.Nil(t, h.comp.done, "preflight mode must not run on an IoT host")
	assert.Empty(t, h.lc.hooks)
}

func TestPreflightModeInertWhenPreflightModeDisabled(t *testing.T) {
	h := newHarness(t, modeNormal, func(cfg pkgconfigmodel.Config) {
		cfg.Set(DataPlanePreflightMode, false, pkgconfigmodel.SourceFile)
	})
	assert.Nil(t, h.comp.done)
	assert.Empty(t, h.lc.hooks)
}

// TestPreflightModeInertWhenBinaryMissing covers the common case on builds that do not ship
// ADP (Heroku, slim container images). It must be silent: reporting it would drown the
// real signal in hosts that were never going to run ADP.
func TestPreflightModeInertWhenBinaryMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "agent-data-plane")
	old := resolveDataPlanePath
	resolveDataPlanePath = func() string { return missing }
	t.Cleanup(func() { resolveDataPlanePath = old })

	cfg := configmock.New(t)
	cfg.Set("run_path", t.TempDir(), pkgconfigmodel.SourceAgentRuntime)
	lc := &testLifecycle{}
	tlm := telemetrymock.New(t)

	p := NewComponent(Requires{Lc: lc, Config: cfg, Log: logmock.New(t), Telemetry: tlm})
	comp, ok := p.Comp.(*preflightModeComponent)
	require.True(t, ok)

	assert.Nil(t, comp.done)
	assert.Empty(t, lc.hooks)
	_, err := tlm.GetCountMetric(telemetrySubsystem, metricResult)
	assert.Error(t, err, "a missing binary must not be reported")
}

// TestPreflightModeReportsUnusableBinary is the flip side: a binary that is present but not
// runnable is a genuine packaging or permissions problem and must be reported.
func TestPreflightModeReportsUnusableBinary(t *testing.T) {
	notExecutable := filepath.Join(t.TempDir(), "agent-data-plane")
	require.NoError(t, os.WriteFile(notExecutable, []byte("not a binary"), 0600))

	old := resolveDataPlanePath
	resolveDataPlanePath = func() string { return notExecutable }
	t.Cleanup(func() { resolveDataPlanePath = old })

	cfg := configmock.New(t)
	cfg.Set("run_path", t.TempDir(), pkgconfigmodel.SourceAgentRuntime)
	lc := &testLifecycle{}
	tlm := telemetrymock.New(t)

	p := NewComponent(Requires{Lc: lc, Config: cfg, Log: logmock.New(t), Telemetry: tlm})
	comp, ok := p.Comp.(*preflightModeComponent)
	require.True(t, ok)

	assert.Nil(t, comp.done)
	metrics, err := tlm.GetCountMetric(telemetrySubsystem, metricResult)
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, string(findingSpawnFailed), metrics[0].Tags()[labelResult])
}

// TestPreflightModeUnbindableSocketPath covers a run_path long enough that the socket cannot be
// bound. Without the up-front check this surfaces from ADP as a bare "invalid argument",
// which is not diagnosable from telemetry alone.
func TestPreflightModeUnbindableSocketPath(t *testing.T) {
	base := shortTempDir(t)
	// Nest until run_path alone is past the limit.
	deep := filepath.Join(base, strings.Repeat("d/", 60))
	require.NoError(t, os.MkdirAll(deep, 0700))

	h := newHarness(t, modeNormal, func(cfg pkgconfigmodel.Config) {
		cfg.Set("run_path", deep, pkgconfigmodel.SourceAgentRuntime)
	})
	h.runToCompletion(t)

	assert.Equal(t, string(findingSpawnFailed), h.result(t))
	assert.Empty(t, h.captured(), "the process must not have been started at all")
}

// TestPreflightModePrepareRestrictsWorkDir checks that prepare hands back a restricted working
// directory with the generated config inside it.
//
// Unix only, because this file is: the fake ADP harness depends on Unix signals. The Windows
// side is covered by TestSecureWorkDir, which does not need the harness.
func TestPreflightModePrepareRestrictsWorkDir(t *testing.T) {
	h := newHarness(t, modeNormal, nil)

	_, _, err := h.comp.prepare()
	require.NoError(t, err)

	workDir := filepath.Join(h.cfg.GetString("run_path"), workDirName)
	cfgPath := filepath.Join(workDir, preflightConfigFileName)

	dirInfo, err := os.Stat(workDir)
	require.NoError(t, err)
	require.True(t, dirInfo.IsDir())

	cfgInfo, err := os.Stat(cfgPath)
	require.NoError(t, err, "the generated config must be inside the secured directory")

	assert.Equal(t, os.FileMode(0700), dirInfo.Mode().Perm(),
		"%s must not be reachable by group or other", workDir)
	assert.Equal(t, os.FileMode(0600), cfgInfo.Mode().Perm(),
		"%s holds the Agent's credentials", cfgPath)
}

// TestPreflightModeAbortsWhenWorkDirCannotBeSecured pins the fail-closed behaviour: a directory we
// could not restrict must stop the pre-flight, not merely be logged. Writing the Agent's
// credentials somewhere unprotected is worse than skipping the run.
func TestPreflightModeAbortsWhenWorkDirCannotBeSecured(t *testing.T) {
	h := newHarness(t, modeNormal, nil)

	original := secureWorkDir
	secureWorkDir = func(string) error { return errors.New("boom") }
	t.Cleanup(func() { secureWorkDir = original })

	h.runToCompletion(t)

	assert.Equal(t, string(findingSpawnFailed), h.result(t))
	assert.Empty(t, h.captured(), "the process must not have been started at all")

	// Nothing may be left behind, least of all a config we could not protect.
	workDir := filepath.Join(h.cfg.GetString("run_path"), workDirName)
	cfgPath := filepath.Join(workDir, preflightConfigFileName)
	_, err := os.Stat(cfgPath)
	assert.Truef(t, os.IsNotExist(err), "%s was written anyway (err=%v)", cfgPath, err)
}

func TestListenerValidate(t *testing.T) {
	assert.NoError(t, newListener("/opt/datadog-agent/run/adp-preflight").validate())
	assert.Error(t, newListener("/"+strings.Repeat("x", maxUnixSocketPath)).validate())
}
