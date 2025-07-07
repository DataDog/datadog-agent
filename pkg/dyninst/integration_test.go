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
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/go-json-experiment/json/jsontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

//go:embed testdata/decoded/*/*.yaml
var testdataFS embed.FS

func TestDyninst(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	programs := testprogs.MustGetPrograms(t)
	for _, svc := range programs {
		if svc == "busyloop" {
			t.Logf("busyloop is not used in integration test")
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
				bin := testprogs.MustGetBinary(t, svc, cfg)

				expectedOutput := getExpectedDecodedOutputOfProbes(t, svc)
				probes := testprogs.MustGetProbeDefinitions(t, svc)
				for i := range probes {
					// Run each probe individually
					t.Run(probes[i].GetID(), func(t *testing.T) {
						t.Parallel()
						testDyninst(t, svc, bin, probes[i:i+1], expectedOutput)
					})
				}
			})
		}
	}
}

func testDyninst(
	t *testing.T,
	service string,
	sampleServicePath string,
	probes []ir.ProbeDefinition,
	expOut map[string]string,
) {
	t.Logf("Testing with actuator")
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

	var sink testMessageSink
	reporter := makeTestReporter(t)
	loader, err := loader.NewLoader(
		// Add following to help debug this test.
		// loader.WithDebugLevel(100),
		loader.WithAdditionalSerializer(&compiler.DebugSerializer{
			Out: codeDump,
		}),
	)
	require.NoError(t, err)
	a, err := actuator.NewActuator(
		actuator.WithMessageSink(&sink),
		actuator.WithReporter(reporter),
		actuator.WithLoader(loader),
	)
	require.NoError(t, err)

	// Launch the sample service.
	t.Logf("launching %s", service)
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, sampleServicePath,
	)

	stat, err := os.Stat(sampleServicePath)
	require.NoError(t, err)
	fileInfo := stat.Sys().(*syscall.Stat_t)
	exe := actuator.Executable{
		Path: sampleServicePath,
		Key: actuator.FileKey{
			FileHandle: actuator.FileHandle{
				Dev: uint64(fileInfo.Dev),
				Ino: fileInfo.Ino,
			},
		},
	}

	// Send update to actuator to instrument the process.
	a.HandleUpdate(actuator.ProcessesUpdate{
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
	<-reporter.attached
	if t.Failed() {
		return
	}

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls")
	sampleStdin.Write([]byte("\n"))

	expNumEvents := len(probes)
	read := []actuator.Message{}
	for m := range sink.ch {
		read = append(read, m)
		if len(read) == expNumEvents {
			break
		}
	}
	require.NoError(t, sampleProc.Wait())

	a.HandleUpdate(actuator.ProcessesUpdate{
		Removals: []actuator.ProcessID{
			{PID: int32(sampleProc.Process.Pid)},
		},
	})
	require.NoError(t, a.Shutdown())

	t.Logf("processing output")
	// TODO: we should intercept raw ringbuf bytes and dump them into tmp dir.
	mef, err := object.NewMMappingElfFile(sampleServicePath)
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
	for _, msg := range read {
		event := decode.Event{
			Event:       msg.Event(),
			ServiceName: service,
		}
		err = decoder.Decode(event, decodeOut)
		require.NoError(t, err)
		t.Logf("Decoded output: \n%s\n", decodeOut.String())
		redacted := redactJSON(t, decodeOut.Bytes())
		t.Logf("Redacted output: \n%s\n", string(redacted))

		outputToCompare := expOut[probes[0].GetID()]
		assert.JSONEq(t, outputToCompare, string(redacted))

		if saveOutput, _ := strconv.ParseBool(os.Getenv("REWRITE")); saveOutput {
			expOut[probes[0].GetID()] = string(redacted)
			saveActualOutputOfProbes(t, service, expOut)
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
	ch  chan actuator.Message
}

func (d *testMessageSink) HandleMessage(m actuator.Message) error {
	d.ch <- m
	return nil
}

func (d *testMessageSink) RegisterProgram(p *ir.Program) {
	d.irp = p
	d.ch = make(chan actuator.Message, 100)
}

func (d *testMessageSink) UnregisterProgram(ir.ProgramID) {
	close(d.ch)
}

type testReporter struct {
	attached chan struct{}
	t        *testing.T
}

// ReportAttached implements actuator.Reporter.
func (r *testReporter) ReportAttached(actuator.ProcessID, *ir.Program) {
	select {
	case r.attached <- struct{}{}:
	default:
	}
}

// ReportDetached implements actuator.Reporter.
func (r *testReporter) ReportDetached(actuator.ProcessID, *ir.Program) {}

// ReportIRGenFailed implements actuator.Reporter.
func (r *testReporter) ReportIRGenFailed(
	programID ir.ProgramID, err error, probes []ir.ProbeDefinition,
) {
	r.t.Fatalf("IR generation failed for program %d: %v (with probes: %v)", programID, err, probes)
}

// ReportLoadingFailed implements actuator.Reporter.
func (r *testReporter) ReportLoadingFailed(program *ir.Program, err error) {
	defer close(r.attached)
	r.t.Fatalf("loading failed for program %d: %v", program.ID, err)
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

// ReportCompilationFailed implements actuator.Reporter.
func (r *testReporter) ReportCompilationFailed(
	programID ir.ProgramID, err error, _ []ir.ProbeDefinition,
) {
	defer close(r.attached)
	r.t.Fatalf("compilation failed for program %d: %v", programID, err)
}

func makeTestReporter(t *testing.T) *testReporter {
	return &testReporter{
		t:        t,
		attached: make(chan struct{}, 1),
	}
}

// getExpectedDecodedOutputOfProbes returns the expected output for a given service.
func getExpectedDecodedOutputOfProbes(t *testing.T, name string) map[string]string {
	expectedOutput := make(map[string]string)
	filename := "testdata/decoded/" + runtime.GOARCH + "/" + name + ".yaml"

	yamlData, err := testdataFS.ReadFile(filename)
	if err != nil {
		t.Errorf("testprogs: %v", err)
		return expectedOutput
	}

	err = yaml.Unmarshal(yamlData, &expectedOutput)
	if err != nil {
		t.Errorf("testprogs: %v", err)
	}
	return expectedOutput
}

// saveActualOutputOfProbes saves the actual output for a given service.
// The output is saved to the expected output directory with the same format as getExpectedDecodedOutputOfProbes.
// Note: This function now saves to the current working directory since embedded files are read-only.
func saveActualOutputOfProbes(t *testing.T, name string, savedState map[string]string) {
	// Create testdata/decoded/{arch} directory if it doesn't exist
	archDir := filepath.Join("testdata", "decoded", runtime.GOARCH)
	err := os.MkdirAll(archDir, 0755)
	if err != nil {
		t.Logf("error creating testdata directory: %s", err)
		return
	}

	filename := filepath.Join(archDir, name+".yaml")
	actualOutputYAML, err := yaml.Marshal(savedState)
	if err != nil {
		t.Logf("error marshaling actual output to YAML: %s", err)
		return
	}
	err = os.WriteFile(filename, actualOutputYAML, 0644)
	if err != nil {
		t.Logf("error writing actual output file: %s", err)
		return
	}
	t.Logf("actual output saved to: %s", filename)
}
