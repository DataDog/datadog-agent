// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s", runtime.GOARCH)
			}
			bin := testprogs.MustGetBinary(t, "simple", cfg)
			testDyninst(t, bin)
		})
	}
}

func testDyninst(t *testing.T, sampleServicePath string) {
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
	t.Logf("launching sample service")
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, sampleServicePath,
	)

	probes := testprogs.MustGetProbeCfgs(t, "simple")
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
	<-reporter.attached
	if t.Failed() {
		return
	}

	// Trigger the function calls, receive the events, and wait for the process
	// to exit.
	t.Logf("Triggering function calls")
	sampleStdin.Write([]byte("\n"))

	// Inlined function is called twice, hence extra event.
	expNumEvents := len(probes) + 1
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

	for _, msg := range read {
		event := msg.Event()
		_, err := bpfOutDump.Write([]byte(event))
		require.NoError(t, err)

		header, err := event.Header()
		require.NoError(t, err)
		require.Equal(t, uint32(len(event)), header.Data_byte_len)
		t.Logf("message header: %#v", *header)

		var i int
		for di, err := range event.DataItems() {
			require.NoError(t, err, "data item %d", i)
			diHeader := di.Header()
			typ, ok := sink.irp.Types[ir.TypeID(diHeader.Type)]
			require.True(t, ok, "unknown type: %d", diHeader.Type)
			if i == 0 {
				require.IsType(t, (*ir.EventRootType)(nil), typ)
			}
			t.Logf(
				"di %d: %s @0x%x: %s",
				i, typ.GetName(), diHeader.Address,
				hex.EncodeToString(di.Data()),
			)
			i++
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
	programID ir.ProgramID, processID actuator.ProcessID, err error,
) {
	defer close(r.attached)
	r.t.Fatalf(
		"attaching failed for program %d to process %d: %v",
		programID, processID, err,
	)
}

// ReportCompilationFailed implements actuator.Reporter.
func (r *testReporter) ReportCompilationFailed(
	programID ir.ProgramID, err error,
) {
	defer close(r.attached)
	r.t.Fatalf("compilation failed for program %d: %v", programID, err)
}

// ReportLoadingFailed implements actuator.Reporter.
func (r *testReporter) ReportLoadingFailed(programID ir.ProgramID, err error) {
	defer close(r.attached)
	r.t.Fatalf("loading failed for program %d: %v", programID, err)
}

func makeTestReporter(t *testing.T) *testReporter {
	return &testReporter{
		t:        t,
		attached: make(chan struct{}, 1),
	}
}

func (r *testReporter) ReportAttached(actuator.ProcessID, []config.Probe) {
	select {
	case r.attached <- struct{}{}:
	default:
	}
}

func (r *testReporter) ReportDetached(actuator.ProcessID, []config.Probe) {}
