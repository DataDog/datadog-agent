// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

//go:embed testdata/decoded
var testdataFS embed.FS

type semaphore chan struct{}

func (s semaphore) acquire() (release func()) {
	s <- struct{}{}
	return func() { <-s }
}

func TestDyninst(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	programs := testprogs.MustGetPrograms(t)
	var integrationTestPrograms = map[string]struct{}{
		"simple": {},
		"sample": {},
	}

	concurrency := max(1, runtime.GOMAXPROCS(0))
	sem := make(semaphore, concurrency)

	// The debug variants of the tests spew logs to the trace_pipe, so we need
	// to clear it after the tests to avoid interfering with other tests.
	// Leave the option to disable this behavior for debugging purposes.
	dontClear, _ := strconv.ParseBool(os.Getenv("DONT_CLEAR_TRACE_PIPE"))
	if !dontClear {
		tp, err := tracefs.OpenFile("trace_pipe", os.O_RDONLY, 0)
		require.NoError(t, err)
		t.Cleanup(func() {
			for {
				deadline := time.Now().Add(100 * time.Millisecond)
				require.NoError(t, tp.SetReadDeadline(deadline))
				n, err := io.Copy(io.Discard, tp)
				require.ErrorIs(t, err, os.ErrDeadlineExceeded)
				if n == 0 {
					break
				}
			}
			require.NoError(t, tp.Close())
		})
	}
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	for _, svc := range programs {
		if _, ok := integrationTestPrograms[svc]; !ok {
			t.Logf("%s is not used in integration tests", svc)
			continue
		}
		for _, cfg := range cfgs {
			t.Run(fmt.Sprintf("%s-%s", svc, cfg), func(t *testing.T) {
				runIntegrationTestSuite(t, svc, cfg, rewrite, sem)
			})
		}
	}
}

func testDyninst(
	t *testing.T,
	service string,
	servicePath string,
	probes []ir.ProbeDefinition,
	rewriteEnabled bool,
	expOut map[string][]json.RawMessage,
	debug bool,
	sem semaphore,
) map[string][]json.RawMessage {
	defer sem.acquire()()
	start := time.Now()
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-integration-test")
	defer cleanup()

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, irDump.Close()) }()

	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, codeDump.Close()) }()

	objectFile, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, objectFile.Close()) }()

	loaderOpts := []loader.Option{
		loader.WithAdditionalSerializer(&compiler.DebugSerializer{
			Out: codeDump,
		}),
	}
	if debug {
		loaderOpts = append(loaderOpts, loader.WithDebugLevel(100))
	}
	reporter := makeTestReporter(t)
	loader, err := loader.NewLoader(loaderOpts...)
	require.NoError(t, err)
	a := actuator.NewActuator(loader)
	require.NoError(t, err)
	at := a.NewTenant("integration-test", reporter, irgen.NewGenerator())

	// Launch the sample service.
	t.Logf("launching %s", service)
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, servicePath,
	)
	defer func() {
		_ = sampleProc.Process.Kill()
		_ = sampleProc.Wait()
	}()

	stat, err := os.Stat(servicePath)
	require.NoError(t, err)
	fileInfo := stat.Sys().(*syscall.Stat_t)
	exe := actuator.Executable{
		Path: servicePath,
		Key: procmon.FileKey{
			FileHandle: procmon.FileHandle{
				Dev: uint64(fileInfo.Dev),
				Ino: fileInfo.Ino,
			},
		},
	}

	// Send update to actuator to instrument the process.
	at.HandleUpdate(actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{
			{
				ProcessID: actuator.ProcessID{
					PID: int32(sampleProc.Process.Pid),
				},
				Executable: exe,
				Probes:     probes,
			},
		},
		Removals: []actuator.ProcessID{},
	})

	// Wait for the process to be attached.
	t.Log("Waiting for attachment")
	sink, ok := <-reporter.attached
	require.True(t, ok)
	if t.Failed() {
		return nil
	}

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls")
	sampleStdin.Write([]byte("\n"))

	var totalExpectedEvents int
	if rewriteEnabled {
		totalExpectedEvents = math.MaxInt
	} else {
		for _, p := range probes {
			totalExpectedEvents += len(expOut[p.GetID()])
		}
	}

	timeout := time.Second
	if !rewriteEnabled {
		// In CI the machines seem to get very overloaded and this takes a
		// shocking amount of time. Given we don't wait for this timeout in
		// the happy path, it's fine to let this be quite long.
		timeout = 5*time.Second + 5*time.Since(start)
	}
	timeoutCh := time.After(timeout)
	var read []output.Event
	var timedOut bool
	for !timedOut && len(read) < totalExpectedEvents {
		select {
		case m := <-sink.ch:
			read = append(read, m)
		case <-timeoutCh:
			timedOut = true
		}
	}
	if !rewriteEnabled && timedOut {
		t.Errorf(
			"timed out after %v waiting for %d events, got %d",
			timeout, totalExpectedEvents, len(read),
		)
	}
	require.NoError(t, sampleProc.Wait())

	at.HandleUpdate(actuator.ProcessesUpdate{
		Removals: []actuator.ProcessID{
			{PID: int32(sampleProc.Process.Pid)},
		},
	})
	require.NoError(t, a.Shutdown())

	t.Logf("processing output")
	// TODO: we should intercept raw ringbuf bytes and dump them into tmp dir.
	obj, err := object.OpenElfFile(servicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	moduledata, err := object.ParseModuleData(obj.Underlying)
	require.NoError(t, err)

	goVersion, err := object.ParseGoVersion(obj.Underlying)
	require.NoError(t, err)

	goDebugSections, err := moduledata.GoDebugSections(obj.Underlying)
	require.NoError(t, err)
	defer func() { require.NoError(t, goDebugSections.Close()) }()

	symbolTable, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	require.NoError(t, err)
	symbolicator := symbol.NewGoSymbolicator(symbolTable)
	require.NotNil(t, symbolicator)

	cachingSymbolicator, err := symbol.NewCachingSymbolicator(symbolicator, 10000)
	require.NotNil(t, symbolicator)
	require.NoError(t, err)

	decoder, err := decode.NewDecoder(sink.irp)
	require.NoError(t, err)

	retMap := make(map[string][]json.RawMessage)
	for _, ev := range read {
		// Validate that the header has the correct program ID.
		{
			header, err := ev.Header()
			require.NoError(t, err)
			require.Equal(t, ir.ProgramID(header.Prog_id), sink.irp.ID)
		}

		event := decode.Event{
			Event:       ev,
			ServiceName: service,
		}
		var decodeOut bytes.Buffer
		probe, err := decoder.Decode(event, cachingSymbolicator, &decodeOut)
		require.NoError(t, err)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Output: %s", decodeOut.String())
		}
		redacted := redactJSON(t, decodeOut.Bytes(), defaultRedactors)
		probeID := probe.GetID()
		probeRet := retMap[probeID]
		expIdx := len(probeRet)
		retMap[probeID] = append(retMap[probeID], json.RawMessage(redacted))
		if expIdx < len(expOut[probeID]) {
			outputToCompare := expOut[probeID][expIdx]
			assert.JSONEq(t, string(outputToCompare), string(redacted))
		}
		if !rewriteEnabled {
			expOut, ok := expOut[probeID]
			assert.True(t, ok, "expected output for probe %s not found", probeID)
			assert.Less(
				t, expIdx, len(expOut),
				"expected at least %d events for probe %s, got %d",
				expIdx+1, probeID, len(expOut),
			)
			assert.Equal(t, string(expOut[expIdx]), string(redacted))
		}
	}
	return retMap
}

type probeOutputs map[string][]json.RawMessage

func runIntegrationTestSuite(
	t *testing.T,
	service string,
	cfg testprogs.Config,
	rewrite bool,
	sem semaphore,
) {
	if cfg.GOARCH != runtime.GOARCH {
		t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
		return
	}
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
	probes := testprogs.MustGetProbeDefinitions(t, service)
	var expectedOutput map[string][]json.RawMessage
	if !rewrite {
		var err error
		expectedOutput, err = getExpectedDecodedOutputOfProbes(service)
		require.NoError(t, err)
	}
	bin := testprogs.MustGetBinary(t, service, cfg)
	for _, debug := range []bool{false, true} {
		runTest := func(t *testing.T, probeSlice []ir.ProbeDefinition) {
			t.Parallel()
			actual := testDyninst(
				t, service, bin, probeSlice, rewrite, expectedOutput, debug, sem,
			)
			if t.Failed() {
				return
			}
			outputs.Lock()
			defer outputs.Unlock()
			outputs.byTest[t.Name()] = actual
		}
		t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			t.Run("all-probes", func(t *testing.T) { runTest(t, probes) })
			for i := range probes {
				probeID := probes[i].GetID()
				t.Run(probeID, func(t *testing.T) {
					runTest(t, probes[i:i+1])
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
			marshaled, err := json.MarshalIndent(out, "", "  ")
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
				"inconsistent output for probe %s in test %s and %s: %s != %s",
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

type testMessageSink struct {
	irp *ir.Program
	ch  chan output.Event
}

func (d *testMessageSink) HandleEvent(ev output.Event) error {
	d.ch <- append(make(output.Event, 0, len(ev)), ev...)
	return nil
}

func (d *testMessageSink) Close() {
	close(d.ch)
}

type testReporter struct {
	attached chan *testMessageSink
	t        *testing.T
	sink     testMessageSink
}

// ReportLoaded implements actuator.Reporter.
func (r *testReporter) ReportLoaded(_ actuator.ProcessID, _ actuator.Executable, p *ir.Program) (actuator.Sink, error) {
	r.sink = testMessageSink{
		irp: p,
		ch:  make(chan output.Event, 100),
	}
	return &r.sink, nil
}

// ReportAttached implements actuator.Reporter.
func (r *testReporter) ReportAttached(actuator.ProcessID, *ir.Program) {
	select {
	case r.attached <- &r.sink:
	default:
	}
}

// ReportDetached implements actuator.Reporter.
func (r *testReporter) ReportDetached(actuator.ProcessID, *ir.Program) {}

// ReportIRGenFailed implements actuator.Reporter.
func (r *testReporter) ReportIRGenFailed(
	processID actuator.ProcessID,
	err error,
	probes []ir.ProbeDefinition,
) {
	r.t.Fatalf(
		"IR generation failed for process %v: %v (with probes: %v)",
		processID, err, probes,
	)
}

// ReportLoadingFailed implements actuator.Reporter.
func (r *testReporter) ReportLoadingFailed(
	processID actuator.ProcessID,
	program *ir.Program,
	err error,
) {
	defer close(r.attached)
	r.t.Fatalf(
		"loading failed for program %d for process %v: %v", program.ID, processID, err,
	)
}

// ReportAttachingFailed implements actuator.Reporter.
func (r *testReporter) ReportAttachingFailed(
	processID actuator.ProcessID, program *ir.Program, err error,
) {
	defer close(r.attached)
	r.t.Fatalf(
		"attaching failed for program %d to process %v: %v",
		program.ID, processID, err,
	)
}

func makeTestReporter(t *testing.T) *testReporter {
	return &testReporter{
		t:        t,
		attached: make(chan *testMessageSink, 1),
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
