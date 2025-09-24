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
	"maps"
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
	"go.uber.org/goleak"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uprobe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	// The debug variants of the tests spew logs to the trace_pipe, so we need
	// to clear it after the tests to avoid interfering with other tests.
	// Leave the option to disable this behavior for debugging purposes.
	dontClear, _ := strconv.ParseBool(os.Getenv("DONT_CLEAR_TRACE_PIPE"))
	if !dontClear {
		t.Logf("clearing trace_pipe!")
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
			t.Logf("closing trace_pipe!")
			require.NoError(t, tp.Close())
		})
	}
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))

	for _, svc := range programs {
		if _, ok := integrationTestPrograms[svc]; !ok {
			t.Logf("%s is not used in integration tests", svc)
			continue
		}
		t.Run(svc, func(t *testing.T) {
			runIntegrationTestSuite(
				t, svc, rewrite, sem, cfgs...,
			)
		})
	}
}

type testRuntime struct {
	irgen      *irgen.Generator
	loader     *loader.Loader
	dispatcher *dispatcher.Dispatcher
	attached   chan *testMessageSink
	t          *testing.T
	irDump     *os.File
}

// Load implements actuator.Runtime.
func (t *testRuntime) Load(
	programID ir.ProgramID,
	executable actuator.Executable,
	processID actuator.ProcessID,
	probes []ir.ProbeDefinition,
) (actuator.LoadedProgram, error) {
	ir, err := t.irgen.GenerateIR(programID, executable.Path, probes)
	if err != nil {
		if t.attached != nil {
			close(t.attached)
		}
		return nil, err
	}
	if t.irDump != nil {
		if yaml, err := irprinter.PrintYAML(ir); err != nil {
			if t.t != nil {
				t.t.Errorf("failed to print IR: %v", err)
			}
		} else {
			if _, err := t.irDump.Write(yaml); err != nil {
				if t.t != nil {
					t.t.Errorf("failed to write IR to file: %v", err)
				}
			}
		}
	}
	smProgram, err := compiler.GenerateProgram(ir)
	if err != nil {
		if t.attached != nil {
			close(t.attached)
		}
		return nil, err
	}
	lp, err := t.loader.Load(smProgram)
	if err != nil {
		if t.attached != nil {
			close(t.attached)
		}
		return nil, err
	}
	sink := &testMessageSink{
		irp: ir,
		ch:  make(chan output.Event, 1000),
	}
	if t.dispatcher != nil {
		t.dispatcher.RegisterSink(ir.ID, sink)
	}
	return &testLoadedProgram{
		lp:         lp,
		ir:         ir,
		executable: executable,
		processID:  processID,
		probes:     probes,
		tr:         t,
		sink:       sink,
		dispatcher: t.dispatcher,
	}, nil
}

func (t *testRuntime) Close() error {
	return nil
}

type testLoadedProgram struct {
	lp         *loader.Program
	ir         *ir.Program
	executable actuator.Executable
	processID  actuator.ProcessID
	probes     []ir.ProbeDefinition
	tr         *testRuntime
	sink       *testMessageSink
	dispatcher *dispatcher.Dispatcher
}

func (t *testLoadedProgram) IR() *ir.Program {
	return t.ir
}

func (t *testLoadedProgram) Attach(
	processID actuator.ProcessID, executable actuator.Executable,
) (actuator.AttachedProgram, error) {
	v, err := uprobe.Attach(t.lp, executable, processID)
	if err != nil {
		log.Errorf("rcscrape: failed to attach to process %v: %v", processID, err)
		close(t.tr.attached)
		return nil, err
	}
	t.tr.attached <- t.sink
	return v, nil
}

func (t *testLoadedProgram) Close() error {
	if t.dispatcher != nil {
		t.dispatcher.UnregisterSink(t.ir.ID)
	} else {
		t.sink.Close()
	}
	t.lp.Close()
	return nil
}

var _ actuator.Runtime = (*testRuntime)(nil)

func testDyninst(
	t *testing.T,
	service string,
	servicePath string,
	probes []ir.ProbeDefinition,
	rewriteEnabled bool,
	expOut map[string][]json.RawMessage,
	debug bool,
	sem dyninsttest.Semaphore,
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
	loader, err := loader.NewLoader(loaderOpts...)
	t.Cleanup(func() { loader.Close() })
	require.NoError(t, err)
	disp := dispatcher.NewDispatcher(loader.OutputReader())
	t.Cleanup(func() { require.NoError(t, disp.Shutdown()) })
	a := actuator.NewActuator()
	t.Cleanup(func() { require.NoError(t, a.Shutdown()) })
	require.NoError(t, err)
	rt := &testRuntime{
		irgen:      irgen.NewGenerator(),
		loader:     loader,
		dispatcher: disp,
		attached:   make(chan *testMessageSink, 1),
		t:          t,
		irDump:     irDump,
	}
	at := a.NewTenant("integration-test", rt)

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
	sink, ok := <-rt.attached
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
	done := false
	for !done && len(read) < totalExpectedEvents {
		select {
		case m, ok := <-sink.ch:
			if !ok {
				done = true
				continue
			}
			read = append(read, m)
		case <-timeoutCh:
			timedOut = true
			done = true
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
	obj, err := object.OpenElfFileWithDwarf(servicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	symbolTable, err := object.ParseGoSymbolTable(obj)
	require.NoError(t, err)
	defer func() { require.NoError(t, symbolTable.Close()) }()
	require.NoError(t, err)
	symbolicator := symbol.NewGoSymbolicator(&symbolTable.GoSymbolTable)
	require.NotNil(t, symbolicator)

	cachingSymbolicator, err := symbol.NewCachingSymbolicator(symbolicator, 10000)
	require.NotNil(t, symbolicator)
	require.NoError(t, err)

	gotypeTable, err := gotype.NewTable(obj)
	require.NoError(t, err)
	defer func() { require.NoError(t, gotypeTable.Close()) }()

	decoder, err := decode.NewDecoder(
		sink.irp, (*decode.GoTypeNameResolver)(gotypeTable),
		time.Now(),
	)
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
		decodeOut := []byte{}
		decodeOut, probe, err := decoder.Decode(event, cachingSymbolicator, decodeOut)
		require.NoError(t, err)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Output: %s", string(decodeOut))
		}
		redacted := redactJSON(t, "", decodeOut, defaultRedactors)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Sorted and redacted: %s", redacted)
		}
		probeID := probe.GetID()
		probeRet := retMap[probeID]
		expIdx := len(probeRet)
		retMap[probeID] = append(retMap[probeID], json.RawMessage(redacted))
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
	rewrite bool,
	sem dyninsttest.Semaphore,
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
	probes := testprogs.MustGetProbeDefinitions(t, service)
	var expectedOutput map[string][]json.RawMessage
	if !rewrite {
		var err error
		expectedOutput, err = getExpectedDecodedOutputOfProbes(service)
		require.NoError(t, err)
	}
	for _, cfg := range cfgs {
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
					actual := testDyninst(
						t, service, bin, probeSlice, rewrite, expectedOutput,
						debug, sem,
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
							func(id string) bool { _, ok := got[id]; return ok },
						)
						require.Empty(
							t, unexpectedProbes,
							"output has probes that are not expected",
						)
					})
					for i := range probes {
						probeID := probes[i].GetID()
						t.Run(probeID, func(t *testing.T) {
							if testing.Short() {
								t.Skip("skipping individual probe with short")
							}
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
