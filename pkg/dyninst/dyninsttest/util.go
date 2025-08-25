// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package dyninsttest provides utilities for dyninst integration testing.
package dyninsttest

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
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

	"github.com/cilium/ebpf/link"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:embed testprogs/testdata/decoded
var testdataFS embed.FS

// MinimumKernelVersion is the minimum kernel version required by the ebpf program.
var MinimumKernelVersion = kernel.VersionCode(5, 17, 0)

// SkipIfKernelNotSupported skips the test if the kernel version is not supported.
func SkipIfKernelNotSupported(t *testing.T) {
	curKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if curKernelVersion < MinimumKernelVersion {
		t.Skipf("Kernel version %v is not supported", curKernelVersion)
	}
}

// Semaphore is a semaphore that can be used to limit the number of concurrent
// operations.
type Semaphore chan struct{}

// MakeSemaphore creates a new semaphore with a number of slots equal to the
// number of CPUs.
func MakeSemaphore() Semaphore {
	return make(Semaphore, max(runtime.GOMAXPROCS(0), 1))
}

// Acquire acquires a slot in the semaphore. It returns a function that must be
// called to release the slot.
func (s Semaphore) Acquire() (release func()) {
	s <- struct{}{}
	return func() { <-s }
}

// SetupLogging is used to have a consistent logging setup for all tests.
// It is best to call this in TestMain.
func SetupLogging() {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "debug"
	}
	const defaultFormat = "%l %Date(15:04:05.000000000) @%File:%Line| %Msg%n"
	var format string
	switch formatFromEnv := os.Getenv("DD_LOG_FORMAT"); formatFromEnv {
	case "":
		format = defaultFormat
	case "json":
		format = `{"time":%Ns,"level":"%Level","msg":"%Msg","path":"%RelFile","func":"%Func","line":%Line}%n`
	case "json-short":
		format = `{"t":%Ns,"l":"%Lev","m":"%Msg"}%n`
	default:
		format = formatFromEnv
	}
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(
		os.Stderr, log.TraceLvl, format,
	)
	if err != nil {
		panic(fmt.Errorf("failed to create logger: %w", err))
	}
	log.SetupLogger(logger, logLevel)
}

// PrepTmpDir creates a temporary directory and a suitable cleanup function.
func PrepTmpDir(t *testing.T, prefix string) (string, func()) {
	dir, err := os.MkdirTemp(os.TempDir(), prefix)
	require.NoError(t, err)
	t.Logf("using temp dir %s", dir)
	return dir, func() {
		preserve, _ := strconv.ParseBool(os.Getenv("KEEP_TEMP"))
		if preserve || t.Failed() {
			t.Logf("leaving temp dir %s for inspection", dir)
		} else {
			require.NoError(t, os.RemoveAll(dir))
		}
	}
}

// GenerateIr generates an IR program based on a binary and a config files.
func GenerateIr(
	t *testing.T,
	tempDir string,
	binPath string,
	cfgName string,
) (*object.ElfFile, *ir.Program) {
	probes := testprogs.MustGetProbeDefinitions(t, cfgName)

	obj, err := object.OpenElfFile(binPath)
	require.NoError(t, err)

	irp, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.Empty(t, irp.Issues)

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { require.NoError(t, irDump.Close()) }()
	irYaml, err := irprinter.PrintYAML(irp)
	require.NoError(t, err)
	_, err = irDump.Write(irYaml)
	require.NoError(t, err)

	return obj, irp
}

// CompileAndLoadBPF compiles an IR program and loads it into the kernel.
func CompileAndLoadBPF(
	t *testing.T,
	tempDir string,
	irp *ir.Program,
) (*loader.Program, func()) {
	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)
	defer func() { require.NoError(t, codeDump.Close()) }()

	smProgram, err := compiler.GenerateProgram(irp)
	require.NoError(t, err)

	loader, err := loader.NewLoader()
	require.NoError(t, err)
	program, err := loader.Load(smProgram)
	require.NoError(t, err)

	return program, func() {
		program.Close()
	}
}

// StartProcess starts a process and returns a write closer for the stdin.
func StartProcess(ctx context.Context, t *testing.T, tempDir string, binPath string, args ...string) (*exec.Cmd, io.WriteCloser) {
	proc := exec.CommandContext(ctx, binPath, args...)
	sampleStdin, err := proc.StdinPipe()
	require.NoError(t, err)
	proc.Stdout, err = os.Create(filepath.Join(tempDir, "sample.out"))
	require.NoError(t, err)
	proc.Stderr, err = os.Create(filepath.Join(tempDir, "sample.err"))
	require.NoError(t, err)
	err = proc.Start()
	require.NoError(t, err)

	require.NoError(t, err)
	return proc, sampleStdin
}

// AttachBPFProbes attaches the BPF program to the running process.
func AttachBPFProbes(
	t *testing.T,
	binPath string,
	obj *object.ElfFile,
	pid int,
	program *loader.Program,
) func() {
	sampleLink, err := link.OpenExecutable(binPath)
	require.NoError(t, err)
	textSection, err := object.FindTextSectionHeader(obj.Underlying.Elf)
	require.NoError(t, err)

	var allAttached []link.Link
	for _, attachpoint := range program.Attachpoints {
		// Despite the name, Uprobe expects an offset in the object file, and not the virtual address.
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		attached, err := sampleLink.Uprobe(
			"",
			program.BpfProgram,
			&link.UprobeOptions{
				PID:     pid,
				Address: addr,
				Offset:  0,
				Cookie:  attachpoint.Cookie,
			},
		)
		require.NoError(t, err)
		allAttached = append(allAttached, attached)
	}
	return func() {
		for _, a := range allAttached {
			require.NoError(t, a.Close())
		}
	}
}

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
	tempDir, cleanup := PrepTmpDir(t, testName)

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

	sampleProc, sampleStdin := StartProcess(ctx, t, env.TempDir, servicePath)

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
	_, err := processInfo.Stdin.Write([]byte("\n"))
	require.NoError(t, err)

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
	Semaphore      Semaphore
	TestNameSuffix string // Optional suffix for test naming (e.g., "-fault-injection")
	TestFunc       func(t *testing.T, service, bin string, probeSlice []ir.ProbeDefinition, rewrite bool, expectedOutput map[string][]json.RawMessage, debug bool, sem Semaphore) map[string][]json.RawMessage
}

type probeOutputs map[string][]json.RawMessage

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
