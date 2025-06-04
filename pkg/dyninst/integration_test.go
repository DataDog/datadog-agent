// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	object "github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
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
	cfgs := testprogs.GetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s", runtime.GOARCH)
			}
			bin := testprogs.GetBinary(t, "events_simple", cfg)
			testDyninst(t, bin)
		})
	}
}

func testDyninst(t *testing.T, sampleServicePath string) {
	t.Logf("loading binary")
	tempDir, err := os.MkdirTemp(os.TempDir(), "dyninst-integration-test-")
	require.NoError(t, err)
	defer func() {
		if t.Failed() {
			t.Logf("leaving temp dir %s for inspection", tempDir)
		} else {
			require.NoError(t, os.RemoveAll(tempDir))
		}
	}()

	// Load the binary and generate the IR.
	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

	probes := []config.Probe{
		&config.LogProbe{
			ID: "intArg",
			Where: &config.Where{
				MethodName: "main.intArg",
			},
		},
	}

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)

	irp, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { require.NoError(t, irDump.Close()) }()
	irYaml, err := irprinter.PrintYAML(irp)
	require.NoError(t, err)
	_, err = irDump.Write(irYaml)
	require.NoError(t, err)

	// Compile the IR and prepare the BPF program.
	t.Logf("compiling BPF")
	codeDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.c"))
	require.NoError(t, err)
	defer func() { require.NoError(t, codeDump.Close()) }()

	bpfObj, err := compiler.CompileBPFProgram(*irp, codeDump)
	require.NoError(t, err)
	defer func() { bpfObj.Close() }()

	bpfObjDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfObjDump.Close()) }()
	_, err = io.Copy(bpfObjDump, bpfObj)
	require.NoError(t, err)

	spec, err := ebpf.LoadCollectionSpecFromReader(bpfObj)
	require.NoError(t, err)

	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{})
	require.NoError(t, err)
	defer func() { bpfCollection.Close() }()

	bpfProg, ok := bpfCollection.Programs["probe_run_with_cookie"]
	require.True(t, ok)

	sampleLink, err := link.OpenExecutable(sampleServicePath)
	require.NoError(t, err)

	// Launch the sample service, inject the BPF program and collect the output.
	t.Logf("running and instrumenting sample")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sampleProc := exec.CommandContext(ctx, sampleServicePath)
	sampleStdin, err := sampleProc.StdinPipe()
	require.NoError(t, err)
	sampleProc.Stdout, err = os.Create(filepath.Join(tempDir, "sample.out"))
	require.NoError(t, err)
	sampleProc.Stderr, err = os.Create(filepath.Join(tempDir, "sample.err"))
	require.NoError(t, err)
	err = sampleProc.Start()
	require.NoError(t, err)

	bpfProbe, err := sampleLink.Uprobe(
		"main.intArg",
		bpfProg,
		&link.UprobeOptions{
			PID: os.Getpid(),
		},
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfProbe.Close()) }()

	attached, err := sampleLink.Uprobe("main.intArg", bpfProg, &link.UprobeOptions{
		PID:    sampleProc.Process.Pid,
		Cookie: 0,
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, attached.Close()) }()

	// Trigger the function calls.
	sampleStdin.Write([]byte("\n"))

	err = sampleProc.Wait()
	require.NoError(t, err)

	// Validate the output. For now we just check the total length.
	t.Logf("processing output")
	rd, err := ringbuf.NewReader(bpfCollection.Maps["out_ringbuf"])
	require.NoError(t, err)

	bpfOutDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.out"))
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfOutDump.Close()) }()

	require.Greater(t, rd.AvailableBytes(), 0)
	record, err := rd.Read()
	require.NoError(t, err)
	bpfOutDump.Write(record.RawSample)

	header := (*output.EventHeader)(unsafe.Pointer(&record.RawSample[0]))
	require.Equal(t, uint32(len(record.RawSample)), header.Data_byte_len)

	pos := uint32(unsafe.Sizeof(*header)) + uint32(header.Stack_byte_len)
	di := (*output.DataItemHeader)(unsafe.Pointer(&record.RawSample[pos]))
	typ, ok := irp.Types[ir.TypeID(di.Type)]
	require.True(t, ok)
	require.IsType(t, &ir.EventRootType{}, typ)
	require.Equal(t, di.Length, typ.GetByteSize())

	expectedTotalLen := uint32(unsafe.Sizeof(*header)) + uint32(header.Stack_byte_len) + uint32(unsafe.Sizeof(*di)) + uint32(di.Length)
	if expectedTotalLen%8 > 0 {
		expectedTotalLen += 8 - expectedTotalLen%8
	}
	require.Equal(t, expectedTotalLen, header.Data_byte_len)
}
