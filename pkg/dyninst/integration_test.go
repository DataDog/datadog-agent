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
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//go:embed testdata/decoded/*.yaml
var testdataFS embed.FS

var MinimumKernelVersion = kernel.VersionCode(5, 17, 0)

func skipIfKernelNotSupported(t *testing.T) {
	curKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if curKernelVersion < MinimumKernelVersion {
		t.Skipf("Kernel version %v is not supported", curKernelVersion)
	}
}

func TestDyninst(t *testing.T) {
	skipIfKernelNotSupported(t)
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
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(
		os.Stderr, log.DebugLvl, "[%LEVEL] %Msg\n",
	)
	require.NoError(t, err)
	log.SetupLogger(logger, "debug")
	t.Logf("Testing with actuator")
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-integration-test")
	defer cleanup()

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, irDump.Close()) }()

	codeDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.c"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, codeDump.Close()) }()

	objectFile, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)
	defer func() { assert.NoError(t, objectFile.Close()) }()

	var sink testMessageSink
	reporter := makeTestReporter(t)
	a, err := actuator.NewActuator(
		actuator.WithMessageSink(&sink),
		actuator.WithReporter(reporter),
		actuator.WithCodegenWriter(func(p *ir.Program) io.Writer {
			yaml, err := irprinter.PrintYAML(p)
			assert.NoError(t, err)
			_, err = io.Copy(irDump, bytes.NewReader(yaml))
			assert.NoError(t, err)
			return codeDump
		}),
		actuator.WithCompiledCallback(func(
			program *actuator.CompiledProgram,
		) {
			// Use a SectionReader to avoid messing with the offset
			// of the underlying io.Reader.
			r := io.NewSectionReader(program.CompiledBPF.Obj, 0, math.MaxInt64)
			_, err = io.Copy(objectFile, r)
			assert.NoError(t, err)
		}),
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
	bpfOutDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.out"))
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfOutDump.Close()) }()

	decoder, err := decode.NewDecoder(sink.irp)
	require.NoError(t, err)
	b := []byte{}
	decodeOut := bytes.NewBuffer(b)
	for _, msg := range read {
		event := msg.Event()
		err = decoder.Decode(event, decodeOut)
		require.NoError(t, err)
		header, err := event.Header()
		require.NoError(t, err)

		// Purge stack fraames
		tmpMap := map[string]any{}
		err = json.Unmarshal(decodeOut.Bytes(), &tmpMap)
		require.NoError(t, err)
		require.Equal(t, uint32(len(event)), header.Data_byte_len)
		t.Logf("message header: %#v", *header)
		if header.Stack_byte_len > 0 {
			stackPCs, err := event.StackPCs()
			require.NoError(t, err)
			t.Logf("stack: %x", stackPCs)
		}

		if _, ok := tmpMap["stack_frames"]; !ok {
			t.Error("No stack frames in output")
		} else {
			tmpMap["stack_frames"] = ""
		}

		clearAddressFields(tmpMap)

		purged, err := json.Marshal(tmpMap)
		assert.NoError(t, err)

		outputToCompare := expOut[probes[0].GetID()]
		assert.JSONEq(t, outputToCompare, string(purged))

		if saveOutput, _ := strconv.ParseBool(os.Getenv("REWRITE")); saveOutput {
			expOut[probes[0].GetID()] = string(purged)
			saveActualOutputOfProbes(t, service, expOut)
		}
	}
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

// ReportLoadingFailed implements actuator.Reporter.
func (r *testReporter) ReportLoadingFailed(program *ir.Program, err error) {
	defer close(r.attached)
	r.t.Fatalf("loading failed for program %d: %v", program.ID, err)
}

func makeTestReporter(t *testing.T) *testReporter {
	return &testReporter{
		t:        t,
		attached: make(chan struct{}, 1),
	}
}

func (r *testReporter) ReportAttached(actuator.ProcessID, *ir.Program) {
	select {
	case r.attached <- struct{}{}:
	default:
	}
}

func (r *testReporter) ReportDetached(actuator.ProcessID, *ir.Program) {}

// clearAddressFields recursively traverses the captures structure and sets all "Address" fields to empty strings.
func clearAddressFields(data map[string]any) {
	if captures, ok := data["captures"]; ok {
		clearAddressFieldsRecursive(captures)
	}
}

// clearAddressFieldsRecursive recursively clears Address fields in any nested structure.
func clearAddressFieldsRecursive(v any) {
	switch val := v.(type) {
	case map[string]any:
		for key, value := range val {
			if key == "Address" {
				val[key] = ""
			} else {
				clearAddressFieldsRecursive(value)
			}
		}
	case []any:
		for _, item := range val {
			clearAddressFieldsRecursive(item)
		}
	}
}

// getExpectedDecodedOutputOfProbes returns the expected output for a given service.
func getExpectedDecodedOutputOfProbes(t *testing.T, name string) map[string]string {
	expectedOutput := make(map[string]string)
	filename := "testdata/decoded/" + name + ".yaml"

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
	// Create testdata/decoded directory if it doesn't exist
	err := os.MkdirAll("testdata/decoded", 0755)
	if err != nil {
		t.Logf("error creating testdata directory: %s", err)
		return
	}

	filename := filepath.Join("testdata", "decoded", name+".yaml")
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
