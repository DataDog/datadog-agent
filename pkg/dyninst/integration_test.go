// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	jsonv2 "github.com/go-json-experiment/json"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

//go:embed testdata/decoded
var testdataFS embed.FS

func TestDyninst(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	current := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, current) })
	cfgs := testprogs.MustGetCommonConfigs(t)
	programs := testprogs.MustGetPrograms(t)
	var integrationTestPrograms = map[string]struct{}{
		"simple": {},
		"sample": {},
		"fault":  {},
	}

	sem := dyninsttest.MakeSemaphore()

	// The debug variants of the tests spew logs to the trace_pipe. We collect
	// these logs organized by PID so they can be output on test failure.
	// Set DONT_COLLECT_TRACE_PIPE=1 to disable collection (e.g., for manual debugging).
	var collector *tracePipeCollector
	dontCollect, _ := strconv.ParseBool(os.Getenv("DONT_COLLECT_TRACE_PIPE"))
	if !dontCollect {
		collector = newTracePipeCollector(t)
		t.Cleanup(func() { collector.Close() })
	}
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))

	for _, svc := range programs {
		if _, ok := integrationTestPrograms[svc]; !ok {
			t.Logf("%s is not used in integration tests", svc)
			continue
		}
		t.Run(svc, func(t *testing.T) {
			runIntegrationTestSuite(
				t, svc, rewrite, sem, collector, cfgs...,
			)
		})
	}
}

func testDyninst(
	t *testing.T,
	service string,
	testProgConfig testprogs.Config,
	servicePath string,
	probes []ir.ProbeDefinition,
	resultNames map[string]string,
	rewriteEnabled bool,
	expOut map[string][]json.RawMessage,
	debug bool,
	sem dyninsttest.Semaphore,
	collector *tracePipeCollector,
) map[string][]json.RawMessage {
	defer sem.Acquire()()
	start := time.Now()
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-integration-test")
	defer cleanup()

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, irDump.Close()) }()

	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, codeDump.Close()) }()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)
	cfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	loaderOpts := []loader.Option{
		loader.WithAdditionalSerializer(&compiler.DebugSerializer{
			Out: codeDump,
		}),
	}
	if debug {
		loaderOpts = append(loaderOpts, loader.WithDebugLevel(100))
	}
	cfg.TestingKnobs.LoaderOptions = loaderOpts
	cfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	cfg.LogUploaderURL = testServer.getLogsURL()
	cfg.DiagsUploaderURL = testServer.getDiagsURL()
	var sendUpdate fakeProcessSubscriber
	cfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close() // prevent start from doing anything
		return &sendUpdate
	}
	cfg.TestingKnobs.IRGeneratorOverride = func(g module.IRGenerator) module.IRGenerator {
		return &outputSavingIRGenerator{irGenerator: g, t: t, output: irDump}
	}
	cfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	cfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &backoff.ExpBackoffPolicy{
			MaxBackoffTime: time.Millisecond.Seconds(),
		},
	}
	cfg.ActuatorConfig.RecompilationRateLimit = -1 // disable recompilation
	m, err := module.NewModule(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	// Launch the sample service.
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, servicePath,
	)
	pid := sampleProc.Process.Pid
	t.Logf("launched %s with pid %d", service, pid)
	defer func() {
		_ = sampleProc.Process.Kill()
		_ = sampleProc.Wait()
	}()

	// On failure in debug mode, output trace_pipe logs for this process.
	t.Cleanup(func() {
		if collector == nil || !t.Failed() || !debug {
			return
		}
		if err := collector.Flush(); err != nil {
			t.Logf("trace pipe flush error: %v", err)
		}
		f, err := collector.GetLogs(pid)
		if err != nil {
			t.Logf("trace pipe GetLogs error: %v", err)
			return
		}
		if f == nil {
			return
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			t.Log(scanner.Text())
		}
	})

	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(sampleProc.Process.Pid))
	require.NoError(t, err)
	const runtimeID = "foo"
	sendUpdate(process.ProcessesUpdate{
		Updates: []process.Config{
			{
				Info: process.Info{
					ProcessID:  process.ID{PID: int32(sampleProc.Process.Pid)},
					Executable: exe,
					Service:    service,
				},
				RuntimeID:         runtimeID,
				Probes:            slices.Clone(probes),
				ShouldUploadSymDB: false,
			},
		},
	})

	// Wait for the process to be attached.
	t.Log("Waiting for attachment")
	allProbeIDs := make(map[string]struct{}, len(probes))
	for _, p := range probes {
		allProbeIDs[p.GetID()] = struct{}{}
	}
	assertProbesInstalled := func(c *assert.CollectT) {
		installedProbeIDs := make(map[string]struct{}, len(probes))
		for _, d := range testServer.getDiags() {
			if d.diagnosticMessage.Debugger.Status == uploader.StatusInstalled {
				installedProbeIDs[d.diagnosticMessage.Debugger.ProbeID] = struct{}{}
			} else if d.diagnosticMessage.Debugger.Status == uploader.StatusError {
				t.Fatalf("probe %s installation failed: %s", d.diagnosticMessage.Debugger.ProbeID, d.diagnosticMessage.Debugger.DiagnosticException.Message)
			}
		}
		assert.Equal(c, allProbeIDs, installedProbeIDs)
	}
	require.EventuallyWithT(
		t, assertProbesInstalled, 180*time.Second, 100*time.Millisecond,
		"diagnostics should indicate that the probes are installed",
	)

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls at %s", time.Now().Format(time.RFC3339))
	sampleStdin.Write([]byte("\n"))

	var totalExpectedEvents int
	if rewriteEnabled {
		totalExpectedEvents = math.MaxInt
	} else {
		for _, p := range probes {
			totalExpectedEvents += len(expOut[resultNames[p.GetID()]])
		}
	}

	timeout := time.Second
	if !rewriteEnabled {
		// In CI the machines seem to get very overloaded and this takes a
		// shocking amount of time. Given we don't wait for this timeout in
		// the happy path, it's fine to let this be quite long.
		timeout = 30*time.Second + 10*time.Since(start)
	}
	deadline := time.Now().Add(timeout)
	var n int
	for time.Now().Before(deadline) {
		if n = len(testServer.getLogs()); n >= totalExpectedEvents {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !rewriteEnabled {
		t.Logf("function calls completed at %s", time.Now().Format(time.RFC3339))
		require.GreaterOrEqual(t, n, totalExpectedEvents, "expected at least %d events, got %d", totalExpectedEvents, n)
	}
	require.NoError(t, sampleProc.Wait())
	sendUpdate(process.ProcessesUpdate{
		Removals: []process.ID{{PID: int32(sampleProc.Process.Pid)}},
	})
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Empty(c, m.DiagnosticsStates())
	}, timeout, 100*time.Millisecond, "expected no diagnostics states")
	m.Close()
	retMap := make(map[string][]json.RawMessage)
	debugEnabled := os.Getenv("DEBUG") != ""
	redactors := append(defaultRedactors[:len(defaultRedactors):len(defaultRedactors)],
		makeRedactorForManyFloats(testProgConfig.GOARCH),
		makeRedactorForFunctionWithChangingState())
	for _, log := range testServer.getLogs() {
		redacted := redactJSON(t, "", log.body, redactors)
		if debugEnabled {
			t.Logf("Output: %v\n", string(log.body))
			t.Logf("Sorted and redacted: %v\n", string(redacted))
		}
		expIdx := len(retMap[log.id])
		id := resultNames[log.id]
		retMap[id] = append(retMap[id], redacted)
		if !rewriteEnabled {
			expOut, ok := expOut[id]
			assert.True(t, ok, "expected output for probe %s not found", id)
			if assert.Less(
				t, expIdx, len(expOut),
				"expected at least %d events for probe %s, got %d",
				expIdx+1, id, len(expOut),
			) {
				assert.Equal(t, string(expOut[expIdx]), string(redacted))
			}
		}
	}
	return retMap
}

type probeOutputs map[string][]json.RawMessage

func runIntegrationTestSuite(
	t *testing.T,
	service string,
	rewrite bool,
	sem dyninsttest.Semaphore,
	collector *tracePipeCollector,
	cfgs ...testprogs.Config,
) {
	var outputs = struct {
		sync.Mutex
		byTest map[string]probeOutputs // testName -> probeID -> [redacted JSON]
	}{
		byTest: make(map[string]probeOutputs),
	}
	if rewrite {
		t.Cleanup(func() {
			if t.Failed() {
				return
			}
			validateAndSaveOutputs(t, service, outputs.byTest)
		})
	}
	var expectedOutput map[string][]json.RawMessage
	if !rewrite {
		var err error
		expectedOutput, err = getExpectedDecodedOutputOfProbes(service)
		require.NoError(t, err)
	}
	// Generally we don't need to run all the tests individually in debug mode.
	// It's useful when debugging an individual probe, but for that you can
	// use this environment variable to run all the tests individually.
	const runAllDebugTestsEnv = "RUN_ALL_DEBUG_TESTS"
	runAllDebugTests, _ := strconv.ParseBool(os.Getenv(runAllDebugTestsEnv))
	for _, cfg := range cfgs {
		probes := testprogs.MustGetProbeDefinitions(t, service)
		probes = slices.DeleteFunc(probes, testprogs.HasIssueTag)
		// Some probes have different output in different versions, due to
		// compiler changes. We rename the probes to organize output into different files.
		resultNames := make(map[string]string)
		otherVersionNames := make(map[string]struct{})
		for _, p := range probes {
			var versions []string
			for _, tag := range p.GetTags() {
				if strings.HasPrefix(tag, "version_diff:") {
					versionDiff := strings.TrimPrefix(tag, "version_diff:")
					versions = append(versions, versionDiff)
					if cfg.GOTOOLCHAIN >= versionDiff {
						resultNames[p.GetID()] = p.GetID() + "_geq_" + versionDiff
						break
					}
				}
			}
			if versions == nil {
				resultNames[p.GetID()] = p.GetID()
				continue
			}
			// Find the largest version diff tag that applies to the version we are running with.
			// Save other variants to recognize unexpected outputs.
			slices.Sort(versions)
			slices.Reverse(versions)
			versions = append(versions, "")
			found := false
			for _, version := range versions {
				var resultName string
				if version == "" {
					resultName = p.GetID()
				} else {
					resultName = p.GetID() + "_geq_" + version
				}
				if !found && cfg.GOTOOLCHAIN >= version {
					resultNames[p.GetID()] = resultName
					found = true
				} else {
					otherVersionNames[resultName] = struct{}{}
				}
			}
		}
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
				return
			}
			t.Parallel()
			bin := testprogs.MustGetBinary(t, service, cfg)
			for _, debug := range []bool{false, true} {
				runTest := func(t *testing.T, probeSlice []ir.ProbeDefinition) map[string][]json.RawMessage {
					t.Parallel()
					probeSlice = slices.Clone(probeSlice)
					actual := testDyninst(
						t, service, cfg, bin, probeSlice, resultNames, rewrite, expectedOutput,
						debug, sem, collector,
					)
					if t.Failed() {
						return nil
					}
					outputs.Lock()
					defer outputs.Unlock()
					outputs.byTest[t.Name()] = actual
					return actual
				}
				t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
					if debug && testing.Short() {
						t.Skip("skipping debug with short")
					}
					t.Parallel()
					t.Run("all-probes", func(t *testing.T) {
						got := runTest(t, probes)
						if got == nil || rewrite || debug {
							return
						}
						// Ensure that we don't have any unexpected probes on
						// disk.
						unexpectedProbes := slices.DeleteFunc(
							slices.Collect(maps.Keys(expectedOutput)),
							func(id string) bool {
								if _, ok := got[id]; ok {
									return true
								}
								if _, ok := otherVersionNames[id]; ok {
									return true
								}
								return false
							},
						)
						require.Empty(
							t, unexpectedProbes,
							"output has probes that are not expected",
						)
					})
					if !runAllDebugTests {
						t.Logf(
							"skipping individual probe debug tests because %s is not set",
							runAllDebugTestsEnv,
						)
						return
					}
					for i := range probes {
						probeID := probes[i].GetID()
						t.Run(probeID, func(t *testing.T) {
							runTest(t, probes[i:i+1])
						})
					}
				})
			}
		})
	}
}

// validateAndSaveOutputs ensures that the outputs for the same probe are consistent
// across all tests and saves them to disk.
func validateAndSaveOutputs(
	t *testing.T, svc string, byTest map[string]probeOutputs,
) {
	byProbe := make(map[string][]byte)
	msgEq := func(a, b json.RawMessage) bool { return bytes.Equal(a, b) }
	findMismatchingTests := func(
		probeID string, cur []json.RawMessage,
	) (testNames []string) {
		for testName, testOutputs := range byTest {
			if out, ok := testOutputs[probeID]; ok {
				if !slices.EqualFunc(out, cur, msgEq) {
					testNames = append(testNames, testName)
				}
			}
		}
		return testNames
	}
	for testName, testOutputs := range byTest {
		for id, out := range testOutputs {
			marshaled, err := jsonv2.Marshal(
				out,
				jsontext.WithIndent("  "),
				jsontext.EscapeForHTML(false),
				jsontext.EscapeForJS(false),
			)
			require.NoError(t, err)
			prev, ok := byProbe[id]
			if !ok {
				byProbe[id] = marshaled
				continue
			}
			if bytes.Equal(prev, marshaled) {
				continue
			}
			otherTestNames := findMismatchingTests(id, out)
			require.Equal(
				t,
				string(prev),
				string(marshaled),
				"inconsistent output for probe %s in test %s and %s",
				id, testName, strings.Join(otherTestNames, ", "),
			)
		}
	}
	for id, out := range byProbe {
		path := getProbeOutputFilename(svc, id)
		if err := saveActualOutputOfProbe(path, out); err != nil {
			t.Logf("error saving actual output for probe %s: %v", id, err)
		} else {
			t.Logf("output saved to: %s", path)
		}
	}
}

func getProbeOutputFilename(service, probeID string) string {
	return filepath.Join(
		"testdata", "decoded", service, probeID+".json",
	)
}

// getExpectedDecodedOutputOfProbes returns the expected output for a given service.
func getExpectedDecodedOutputOfProbes(progName string) (map[string][]json.RawMessage, error) {
	dir := filepath.Join("testdata", "decoded", progName)
	entries, err := testdataFS.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	expected := make(map[string][]json.RawMessage)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		probeID := strings.TrimSuffix(e.Name(), ".json")
		content, err := testdataFS.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}
		var out []json.RawMessage
		if err := json.Unmarshal(content, &out); err != nil {
			return nil, fmt.Errorf("unmarshalling %s: %w", e.Name(), err)
		}
		expected[probeID] = out
	}
	return expected, nil
}

// saveActualOutputOfProbes saves the actual output for a given service.
// The output is saved to the expected output directory with the same format as getExpectedDecodedOutputOfProbes.
// Note: This function now saves to the current working directory since embedded files are read-only.
func saveActualOutputOfProbe(outputPath string, content []byte) error {
	outputDir := path.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("error creating testdata directory: %w", err)
	}

	baseName := path.Base(outputPath)
	tmpFile, err := os.CreateTemp(outputDir, "."+baseName+".*.tmp.json")
	if err != nil {
		return fmt.Errorf("error creating temp output file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmpFile, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("error writing temp output: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("error closing temp output: %w", err)
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return fmt.Errorf("error renaming temp output: %w", err)
	}
	return nil
}

// outputSavingIRGenerator is an IRGenerator that saves the output to a file.
type outputSavingIRGenerator struct {
	irGenerator module.IRGenerator
	t           *testing.T
	output      *os.File
}

// GenerateIR implements module.IRGenerator.
func (o *outputSavingIRGenerator) GenerateIR(
	programID ir.ProgramID, binaryPath string, probes []ir.ProbeDefinition, options ...irgen.Option,
) (*ir.Program, error) {
	ir, err := o.irGenerator.GenerateIR(programID, binaryPath, probes, options...)
	if err != nil {
		return nil, err
	}
	assert.NoError(o.t, func() error {
		irYaml, err := irprinter.PrintYAML(ir)
		if err != nil {
			return fmt.Errorf("error printing IR: %w", err)
		}
		_, err = o.output.Write(irYaml)
		if err != nil {
			return fmt.Errorf("error writing IR: %w", err)
		}
		return nil
	}(), "error saving IR")
	return ir, nil

}

var _ module.IRGenerator = (*outputSavingIRGenerator)(nil)

type fakeAgent struct {
	s  *httptest.Server
	t  *testing.T
	mu struct {
		sync.Mutex
		logs  []receivedLog
		diags []receivedDiag
	}
}

const (
	logPath  = "/logs"
	diagPath = "/diags"
)

func newFakeAgent(t *testing.T) *fakeAgent {
	f := &fakeAgent{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/logs", http.HandlerFunc(f.handleLogsUpload))
	mux.HandleFunc("/diags", http.HandlerFunc(f.handleDiagsUpload))
	f.s = httptest.NewServer(mux)
	return f
}

func (f *fakeAgent) getLogsURL() string  { return f.s.URL + logPath }
func (f *fakeAgent) getDiagsURL() string { return f.s.URL + diagPath }

type receivedLog struct {
	id        string
	timestamp int64
	body      json.RawMessage
	headers   http.Header
}

func (f *fakeAgent) getLogs() []receivedLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mu.logs
}

type receivedDiag struct {
	headers           http.Header
	diagnosticMessage *uploader.DiagnosticMessage
}

func (f *fakeAgent) getDiags() []receivedDiag {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mu.diags
}

func (f *fakeAgent) handleLogsUpload(w http.ResponseWriter, req *http.Request) {
	logs, err := readLogs(req)
	if err != nil {
		f.t.Errorf("failed to read logs: %v", err)
		http.Error(w, "failed to read logs", http.StatusBadRequest)
		return
	}
	defer w.WriteHeader(http.StatusOK)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mu.logs = append(f.mu.logs, logs...)
}

func (f *fakeAgent) handleDiagsUpload(w http.ResponseWriter, req *http.Request) {
	diags, err := readDiags(req)
	if err != nil {
		f.t.Errorf("failed to read diags: %v", err)
		http.Error(w, "failed to read diags", http.StatusBadRequest)
		return
	}
	defer w.WriteHeader(http.StatusOK)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mu.diags = append(f.mu.diags, diags...)
}

func readLogs(req *http.Request) ([]receivedLog, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	var rawLogs []json.RawMessage
	if err := json.Unmarshal(body, &rawLogs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal body: %w", err)
	}
	type log struct {
		Debugger struct {
			Snapshot struct {
				Timestamp int64 `json:"timestamp"`
				Probe     struct {
					ID string `json:"id"`
				} `json:"probe"`
			} `json:"snapshot"`
		} `json:"debugger"`
	}
	ret := make([]receivedLog, len(rawLogs))
	for i, raw := range rawLogs {
		var l log
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("failed to unmarshal log: %w", err)
		}
		ret[i] = receivedLog{
			id:        l.Debugger.Snapshot.Probe.ID,
			timestamp: l.Debugger.Snapshot.Timestamp,
			body:      raw,
			headers:   req.Header,
		}
	}
	return ret, nil
}

func readDiags(req *http.Request) ([]receivedDiag, error) {
	file, _, err := req.FormFile("event")
	if err != nil {
		return nil, fmt.Errorf("failed to get event file: %w", err)
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var rawDiags []*uploader.DiagnosticMessage
	if err := json.Unmarshal(body, &rawDiags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal body: %w", err)
	}
	ret := make([]receivedDiag, len(rawDiags))
	for i, raw := range rawDiags {
		ret[i] = receivedDiag{
			headers:           req.Header,
			diagnosticMessage: raw,
		}
	}
	return ret, nil
}

type fakeProcessSubscriber func(process.ProcessesUpdate)

func (f *fakeProcessSubscriber) Subscribe(cb func(process.ProcessesUpdate)) {
	*f = cb
}
func (f *fakeProcessSubscriber) Start() {}

// Make a redactor for the onlyOnAmd64_17 float
// return value in the testReturnsManyFloats function. This function is expected
// to have different behavior based on the architecture, and we need to
// compensate for that.
func makeRedactorForManyFloats(arch string) jsonRedactor {
	return redactor(
		exactMatcher(
			"/debugger/snapshot/captures/return/locals/onlyOnAmd64_16",
		),
		replacerFunc(func(v jsontext.Value) jsontext.Value {
			// If we've already redacted this, don't do it again.
			if v.Kind() == '"' {
				return v
			}
			// Try to read the value.
			var value struct {
				Value             string `json:"value"`
				NotCapturedReason string `json:"notCapturedReason"`
			}
			if err := json.Unmarshal(v, &value); err != nil {
				marshaled, _ := json.Marshal(err.Error())
				return jsontext.Value(marshaled)
			}
			if arch == "amd64" {
				if value.Value != "16" {
					return v
				}
			} else {
				if value.NotCapturedReason != "unavailable" {
					return v
				}
			}
			return jsontext.Value(`"[onlyOnAmd64]"`)
		}),
	)
}

func makeRedactorForFunctionWithChangingState() jsonRedactor {
	return redactor(
		matchRegexp(
			"/debugger/snapshot/captures/lines/.*/locals/aPerArch/value",
		),
		replacement(`"[per-arch-value]"`),
	)
}
