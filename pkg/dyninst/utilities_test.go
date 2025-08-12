// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

// TestEnvironment holds the common test setup artifacts that both
// regular and fault injection tests need.
type TestEnvironment struct {
	TempDir    string
	IRDump     *os.File
	CodeDump   *os.File
	ObjectFile *os.File
	Cleanup    func()
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
	irDump   *os.File
}

// ReportLoaded implements actuator.Reporter.
func (r *testReporter) ReportLoaded(_ actuator.ProcessID, _ actuator.Executable, p *ir.Program) (actuator.Sink, error) {
	if yaml, err := irprinter.PrintYAML(p); err != nil {
		r.t.Errorf("failed to print IR: %v", err)
	} else if _, err := io.Copy(r.irDump, bytes.NewReader(yaml)); err != nil {
		r.t.Errorf("failed to write IR to file: %v", err)
	}
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
	defer close(r.attached)
	r.t.Fatalf(
		"IR generation failed for process %v: %#+v (with probes: %v)",
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

func makeTestReporter(t *testing.T, irDump *os.File) *testReporter {
	return &testReporter{
		t:        t,
		attached: make(chan *testMessageSink, 1),
		irDump:   irDump,
	}
}

// prepareTestEnvironment sets up the common test environment with temp directories
// and output files that both test types use.
func prepareTestEnvironment(t *testing.T, testName string) *TestEnvironment {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, testName)

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)

	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)

	objectFile, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)

	return &TestEnvironment{
		TempDir:    tempDir,
		IRDump:     irDump,
		CodeDump:   codeDump,
		ObjectFile: objectFile,
		Cleanup: func() {
			assert.NoError(t, irDump.Close())
			assert.NoError(t, codeDump.Close())
			assert.NoError(t, objectFile.Close())
			cleanup()
		},
	}
}

// actuatorConfig holds the configuration for creating an actuator
// with appropriate loader options.
type actuatorConfig struct {
	Debug bool
}

// createActuatorWithTenant creates a configured actuator and tenant for testing.
// This abstracts the common actuator setup used by both test types.
func createActuatorWithTenant(t *testing.T, env *TestEnvironment, config actuatorConfig) (*actuator.Actuator, actuator.Tenant, *testReporter) {
	loaderOpts := []loader.Option{
		loader.WithAdditionalSerializer(&compiler.DebugSerializer{
			Out: env.CodeDump,
		}),
	}
	if config.Debug {
		loaderOpts = append(loaderOpts, loader.WithDebugLevel(100))
	}

	reporter := makeTestReporter(t, env.IRDump)
	loader, err := loader.NewLoader(loaderOpts...)
	require.NoError(t, err)

	a := actuator.NewActuator(loader)
	require.NoError(t, err)

	at := a.NewTenant("integration-test", reporter, irgen.NewGenerator())

	return a, *at, reporter
}

// processInfo holds information about a launched test process.
type processInfo struct {
	Process    *os.Process
	Stdin      io.WriteCloser
	Executable actuator.Executable
}

// launchTestProcess starts a test service process and prepares it for instrumentation.
// This handles the common process launching logic used by both test types.
func launchTestProcess(ctx context.Context, t *testing.T, env *TestEnvironment, service, servicePath string) *processInfo {
	t.Logf("launching %s", service)

	sampleProc, sampleStdin := dyninsttest.StartProcess(ctx, t, env.TempDir, servicePath)

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

	return &processInfo{
		Process:    sampleProc.Process,
		Stdin:      sampleStdin,
		Executable: exe,
	}
}

// instrumentProcess sends instrumentation update to the actuator for the given process.
func instrumentProcess(tenant actuator.Tenant, processInfo *processInfo, probes []ir.ProbeDefinition) {
	tenant.HandleUpdate(actuator.ProcessesUpdate{
		Processes: []actuator.ProcessUpdate{
			{
				ProcessID: actuator.ProcessID{
					PID: int32(processInfo.Process.Pid),
				},
				Executable: processInfo.Executable,
				Probes:     probes,
			},
		},
		Removals: []actuator.ProcessID{},
	})
}

// eventCollectionConfig configures how events are collected during testing.
type eventCollectionConfig struct {
	RewriteEnabled      bool
	ExpectedEventCounts map[string]int // probeID -> expected count
	StartTime           time.Time
}

// waitForAttachmentAndCollectEvents handles the common pattern of waiting for
// process attachment, triggering function calls, and collecting events.
func waitForAttachmentAndCollectEvents(
	t *testing.T,
	reporter *testReporter,
	processInfo *processInfo,
	config eventCollectionConfig,
) ([]output.Event, *testMessageSink) {
	// Wait for the process to be attached
	t.Log("Waiting for attachment")
	sink, ok := <-reporter.attached
	require.True(t, ok)
	if t.Failed() {
		return nil, nil
	}

	// Trigger the function calls
	t.Logf("Triggering function calls")
	processInfo.Stdin.Write([]byte("\n"))

	// Calculate expected events
	var totalExpectedEvents int
	if config.RewriteEnabled {
		totalExpectedEvents = math.MaxInt
	} else {
		for _, count := range config.ExpectedEventCounts {
			totalExpectedEvents += count
		}
	}

	// Set up timeout
	timeout := time.Second
	if !config.RewriteEnabled {
		// In CI the machines seem to get very overloaded and this takes a
		// shocking amount of time. Given we don't wait for this timeout in
		// the happy path, it's fine to let this be quite long.
		timeout = 5*time.Second + 5*time.Since(config.StartTime)
	}

	// Collect events
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

	if !config.RewriteEnabled && timedOut {
		t.Errorf(
			"timed out after %v waiting for %d events, got %d",
			timeout, totalExpectedEvents, len(read),
		)
	}

	return read, sink
}

// CleanupProcess handles the common actuator cleanup pattern.
// Note: This function assumes the process has already been waited for.
func cleanupProcess(t *testing.T, processInfo *processInfo, tenant actuator.Tenant, act *actuator.Actuator) {
	tenant.HandleUpdate(actuator.ProcessesUpdate{
		Removals: []actuator.ProcessID{
			{PID: int32(processInfo.Process.Pid)},
		},
	})
	require.NoError(t, act.Shutdown())
}

// goSymbolicatorWithCleanup holds a symbolicator and the resources that need cleanup.
type goSymbolicatorWithCleanup struct {
	Symbolicator    symbol.Symbolicator
	goDebugSections *object.GoDebugSections
	obj             *object.ElfFile
}

// close cleans up the resources associated with the symbolicator.
func (g *goSymbolicatorWithCleanup) close() error {
	if err := g.goDebugSections.Close(); err != nil {
		return err
	}
	return g.obj.Close()
}

// createGoSymbolicator creates a standard Go symbolicator from a service path.
// This abstracts the common symbol table setup used by regular tests.
// The caller must call Close() on the returned object to clean up resources.
func createGoSymbolicator(t *testing.T, servicePath string) *goSymbolicatorWithCleanup {
	obj, err := object.OpenElfFile(servicePath)
	require.NoError(t, err)

	moduledata, err := object.ParseModuleData(obj.Underlying)
	require.NoError(t, err)

	goVersion, err := object.ReadGoVersion(obj.Underlying)
	require.NoError(t, err)

	goDebugSections, err := moduledata.GoDebugSections(obj.Underlying)
	require.NoError(t, err)

	symbolTable, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data(),
		goDebugSections.GoFunc.Data(),
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

	return &goSymbolicatorWithCleanup{
		Symbolicator:    cachingSymbolicator,
		goDebugSections: goDebugSections,
		obj:             obj,
	}
}

// FaultySymbolicator is a symbolicator that always returns an error.
// This is used in fault injection tests to test error handling.
type FaultySymbolicator struct{}

// Symbolicate always returns an error for testing fault injection scenarios.
func (s *FaultySymbolicator) Symbolicate(_ []uint64, _ uint64) ([]symbol.StackFrame, error) {
	return nil, fmt.Errorf("error symbolicating stack")
}

// injectMissingDecoderType removes decoder types for testing fault injection scenarios.
func injectMissingDecoderType(program *ir.Program, ev output.Event) error {
	for item, err := range ev.DataItems() {
		if err != nil {
			return err
		}
		_, ok := program.Types[ir.TypeID(item.Header().Type)].(*ir.EventRootType)
		if !ok {
			delete(program.Types, ir.TypeID(item.Header().Type))
		}
	}
	return nil
}

// EventProcessingConfig configures how events are processed and decoded.
type EventProcessingConfig struct {
	Service        string
	RewriteEnabled bool
	ExpectedOutput map[string][]json.RawMessage
	ProbeIDPrefix  string // Optional prefix for probe IDs (e.g., "faulty-")
}

// processAndDecodeEvents handles the common pattern of processing collected events,
// decoding them, and validating against expected output.
func processAndDecodeEvents(
	t *testing.T,
	events []output.Event,
	sink *testMessageSink,
	symbolicator symbol.Symbolicator,
	config EventProcessingConfig,
) map[string][]json.RawMessage {
	t.Logf("processing output")

	decoder, err := decode.NewDecoder(sink.irp)
	require.NoError(t, err)

	retMap := make(map[string][]json.RawMessage)
	for _, ev := range events {
		// Validate that the header has the correct program ID
		{
			header, err := ev.Header()
			require.NoError(t, err)
			require.Equal(t, ir.ProgramID(header.Prog_id), sink.irp.ID)
		}

		event := decode.Event{
			Event:       ev,
			ServiceName: config.Service,
		}

		var (
			probe ir.ProbeDefinition
			err   error
		)
		output := bytes.NewBuffer([]byte{})
		probe, err = decoder.Decode(event, symbolicator, output)
		require.NoError(t, err)

		if os.Getenv("DEBUG") != "" {
			t.Logf("Output: %s", output.String())
		}

		redacted := redactJSON(t, "", output.Bytes(), defaultRedactors)
		if os.Getenv("DEBUG") != "" {
			t.Logf("Sorted and redacted: %s", redacted)
		}

		probeID := probe.GetID()
		if config.ProbeIDPrefix != "" {
			probeID = config.ProbeIDPrefix + probeID
		}

		probeRet := retMap[probeID]
		expIdx := len(probeRet)
		retMap[probeID] = append(retMap[probeID], json.RawMessage(redacted))

		if !config.RewriteEnabled {
			expOut, ok := config.ExpectedOutput[probeID]
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

// RunTestSuiteConfig configures how test suites are executed.
type RunTestSuiteConfig struct {
	Service        string
	Config         testprogs.Config
	Rewrite        bool
	Semaphore      dyninsttest.Semaphore
	TestNameSuffix string // Optional suffix for test naming (e.g., "-fault-injection")
	TestFunc       func(t *testing.T, service, bin string, probeSlice []ir.ProbeDefinition, rewrite bool, expectedOutput map[string][]json.RawMessage, debug bool, sem dyninsttest.Semaphore) map[string][]json.RawMessage
}

// RunIntegrationTestSuite provides the common test suite execution pattern
// used by both regular and fault injection tests.
func RunIntegrationTestSuite(t *testing.T, config RunTestSuiteConfig) {
	if config.Config.GOARCH != runtime.GOARCH {
		t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, config.Config.GOARCH)
		return
	}

	var outputs = struct {
		sync.Mutex
		byTest map[string]probeOutputs // testName -> probeID -> [redacted JSON]
	}{
		byTest: make(map[string]probeOutputs),
	}

	if config.Rewrite {
		t.Cleanup(func() {
			if t.Failed() {
				return
			}
			validateAndSaveOutputs(t, config.Service, outputs.byTest)
		})
	}

	probes := testprogs.MustGetProbeDefinitions(t, config.Service)
	var expectedOutput map[string][]json.RawMessage
	if !config.Rewrite {
		var err error
		expectedOutput, err = getExpectedDecodedOutputOfProbes(config.Service)
		require.NoError(t, err)
	}

	bin := testprogs.MustGetBinary(t, config.Service, config.Config)

	for _, debug := range []bool{false, true} {
		runTest := func(t *testing.T, probeSlice []ir.ProbeDefinition) {
			t.Parallel()
			actual := config.TestFunc(
				t, config.Service, bin, probeSlice, config.Rewrite, expectedOutput, debug, config.Semaphore,
			)
			if t.Failed() {
				return
			}
			outputs.Lock()
			defer outputs.Unlock()
			outputs.byTest[t.Name()] = actual
		}

		debugSuffix := fmt.Sprintf("debug=%t", debug)
		if config.TestNameSuffix != "" {
			debugSuffix += config.TestNameSuffix
		}

		t.Run(debugSuffix, func(t *testing.T) {
			if debug && testing.Short() {
				t.Skip("skipping debug with short")
			}
			t.Parallel()
			t.Run("all-probes", func(t *testing.T) { runTest(t, probes) })
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
}
