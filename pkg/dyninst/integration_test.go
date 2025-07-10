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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

//go:embed testdata/decoded/*/*.yaml
var testdataFS embed.FS

func TestDyninst(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	programs := testprogs.MustGetPrograms(t)
	var integrationTestPrograms = map[string]struct{}{
		"simple": {},
		"sample": {},
	}

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
	for _, svc := range programs {
		if _, ok := integrationTestPrograms[svc]; !ok {
			t.Logf("%s is not used in integration tests", svc)
			continue
		}
		for _, cfg := range cfgs {
			t.Run(svc+"-"+cfg.String(), func(t *testing.T) {
				if cfg.GOARCH != runtime.GOARCH {
					t.Skipf(
						"cross-execution is not supported, running on %s",
						runtime.GOARCH,
					)
				}

				t.Parallel()
				bin := testprogs.MustGetBinary(t, svc, cfg)

				expectedOutput, err := getExpectedDecodedOutputOfProbes(svc)
				require.NoError(t, err)
				probes := testprogs.MustGetProbeDefinitions(t, svc)
				for _, debug := range []bool{false, true} {
					if testing.Short() && debug {
						t.Skip("skipping debug mode in short mode")
					}
					t.Run(fmt.Sprintf("debug=%t", debug), func(t *testing.T) {
						t.Parallel()
						for i := range probes {
							probes := probes[i : i+1]
							probe := probes[0]
							// Run each probe individually.
							t.Run(probe.GetID(), func(t *testing.T) {
								t.Parallel()
								testDyninst(
									t, svc, bin, probes, expectedOutput, debug,
								)
							})
						}
					})
				}
			})
		}
	}
}

func testDyninst(
	t *testing.T,
	service string,
	servicePath string,
	probes []ir.ProbeDefinition,
	expOut map[string]string,
	debug bool,
) {
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
	at := a.NewTenant("integration-test", reporter)

	// Launch the sample service.
	t.Logf("launching %s", service)
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, servicePath,
	)

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
		return
	}

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls")
	sampleStdin.Write([]byte("\n"))

	expNumEvents := len(probes)
	var read []output.Event
	for m := range sink.ch {
		read = append(read, m)
		if len(read) == expNumEvents {
			break
		}
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
	mef, err := object.NewMMappingElfFile(servicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, mef.Close()) }()
	obj, err := object.NewElfObject(mef.Elf)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	moduledata, err := object.ParseModuleData(mef)
	require.NoError(t, err)

	goVersion, err := object.ParseGoVersion(mef)
	require.NoError(t, err)

	goDebugSections, err := moduledata.GoDebugSections(mef)
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

	decoder, err := decode.NewDecoder(sink.irp, symbolicator)
	require.NoError(t, err)
	b := []byte{}
	decodeOut := bytes.NewBuffer(b)
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
		err = decoder.Decode(event, decodeOut)
		require.NoError(t, err)
		t.Logf("decoded output: \n%s\n", decodeOut.String())
		redacted := redactJSON(t, decodeOut.Bytes())
		t.Logf("redacted output: \n%s\n", string(redacted))

		outputToCompare := expOut[probes[0].GetID()]
		assert.JSONEq(t, outputToCompare, string(redacted))

		if saveOutput, _ := strconv.ParseBool(os.Getenv("REWRITE")); saveOutput {
			expOut[probes[0].GetID()] = string(redacted)
			outputName := getOutputFilename(service)
			if err := saveActualOutputOfProbes(outputName, expOut); err != nil {
				t.Logf("error saving actual output: %s", err)
			} else {
				t.Logf("output saved to: %s", outputName)
			}
		}
	}
}

type jsonRedactor struct {
	matches     func(ptr jsontext.Pointer) bool
	replacement jsontext.Value
}

func redactPtr(toMatch string, replacement jsontext.Value) jsonRedactor {
	return jsonRedactor{
		matches: func(ptr jsontext.Pointer) bool {
			return string(ptr) == toMatch
		},
		replacement: replacement,
	}
}

func redactPtrPrefixSuffix(prefix, suffix string, replacement jsontext.Value) jsonRedactor {
	return jsonRedactor{
		matches: func(ptr jsontext.Pointer) bool {
			return strings.HasPrefix(string(ptr), prefix) && strings.HasSuffix(string(ptr), suffix)
		},
		replacement: replacement,
	}
}

func redactPtrRegexp(pat string, replacement jsontext.Value) jsonRedactor {
	re := regexp.MustCompile(pat)
	return jsonRedactor{
		matches: func(ptr jsontext.Pointer) bool {
			return re.MatchString(string(ptr))
		},
		replacement: replacement,
	}
}

var redactors = []jsonRedactor{
	redactPtrRegexp(`^/debugger/snapshot/stack/[0-9]+/fileName$`, []byte(`"<fileName>"`)),
	redactPtrRegexp(`^/debugger/snapshot/stack/[0-9]+/lineNumber$`, []byte(`"<lineNumber>"`)),

	// TODO: stack is only redacted in full because of bug with stack walking on arm64.
	// Unredact this once the issue is fixed.
	redactPtr(`/debugger/snapshot/stack`, []byte(`"<stack-unredact-me>"`)),

	redactPtr("/debugger/snapshot/id", []byte(`"<id>"`)),
	redactPtr("/debugger/snapshot/timestamp", []byte(`"<ts>"`)),
	redactPtrPrefixSuffix("/debugger/snapshot/captures/", "/address", []byte(`"<addr>"`)),
}

func redactJSON(t *testing.T, input []byte) (redacted []byte) {
	d := jsontext.NewDecoder(bytes.NewReader(input))
	var buf bytes.Buffer
	e := jsontext.NewEncoder(&buf)
	for {
		tok, err := d.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		kind, idx := d.StackIndex(d.StackDepth())
		require.NoError(t, e.WriteToken(tok))
		if kind != '{' || idx%2 == 0 {
			continue
		}
		ptr := d.StackPointer()
		for _, redactor := range redactors {
			if redactor.matches(ptr) {
				_, err := d.ReadValue()
				require.NoError(t, err)
				require.NoError(t, e.WriteValue(redactor.replacement))
				break
			}
		}
	}
	return buf.Bytes()
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

func getOutputFilename(progName string) string {
	return filepath.Join(
		"testdata", "decoded", runtime.GOARCH, progName+".yaml",
	)
}

// getExpectedDecodedOutputOfProbes returns the expected output for a given service.
func getExpectedDecodedOutputOfProbes(progName string) (map[string]string, error) {
	expectedOutput := make(map[string]string)
	outputName := getOutputFilename(progName)
	yamlData, err := testdataFS.ReadFile(outputName)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlData, &expectedOutput)
	if err != nil {
		return nil, err
	}
	return expectedOutput, nil
}

// saveActualOutputOfProbes saves the actual output for a given service.
// The output is saved to the expected output directory with the same format as getExpectedDecodedOutputOfProbes.
// Note: This function now saves to the current working directory since embedded files are read-only.
func saveActualOutputOfProbes(outputPath string, savedState map[string]string) error {
	outputDir := path.Dir(outputPath)
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating testdata directory: %w", err)
	}
	actualOutputYAML, err := yaml.Marshal(savedState)
	if err != nil {
		return fmt.Errorf("error marshaling actual output to YAML: %w", err)
	}
	baseName := path.Base(outputPath)
	tmpFile, err := os.CreateTemp(outputDir, "."+baseName+".*.tmp.yaml")
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}
	tmpFileName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpFileName) }() // remove it if not renamed
	if _, err := io.Copy(tmpFile, bytes.NewReader(actualOutputYAML)); err != nil {
		return fmt.Errorf("error writing actual output file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("error closing output file: %w", err)
	}
	if err := os.Rename(tmpFileName, outputPath); err != nil {
		return fmt.Errorf("error renaming output file: %w", err)
	}
	return nil
}
