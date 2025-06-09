// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-integration-test-")
	defer cleanup()

	// Load the binary and generate the IR.
	t.Logf("loading binary")
	obj, irp := dyninsttest.GenerateIr(t, tempDir, sampleServicePath, "simple")

	// Compile the IR and prepare the BPF program.
	t.Logf("compiling BPF")
	bpfCollection, bpfProg, attachpoints, cleanup := dyninsttest.CompileAndLoadBPF(t, tempDir, irp)
	defer cleanup()

	// Launch the sample service, inject the BPF program and collect the output.
	t.Logf("running and instrumenting sample")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sampleProc, sampleStdin := dyninsttest.StartProcess(ctx, t, tempDir, sampleServicePath)
	cleanup = dyninsttest.AttachBPFProbes(t, sampleServicePath, obj, sampleProc.Process.Pid, bpfProg, attachpoints)
	defer cleanup()

	// Trigger the function calls and wait for the process to exit.
	sampleStdin.Write([]byte("\n"))
	err := sampleProc.Wait()
	require.NoError(t, err)

	// Validate the output. For now we just check the total length.
	t.Logf("processing output")
	rd, err := ringbuf.NewReader(bpfCollection.Maps["out_ringbuf"])
	require.NoError(t, err)

	bpfOutDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.out"))
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfOutDump.Close()) }()

	// Inlined function is called twice, hence extra event.
	for range len(irp.Probes) + 1 {
		t.Logf("reading ringbuf item")
		require.Greater(t, rd.AvailableBytes(), 0)
		record, err := rd.Read()
		require.NoError(t, err)
		bpfOutDump.Write(record.RawSample)

		header := (*output.EventHeader)(unsafe.Pointer(&record.RawSample[0]))
		require.Equal(t, uint32(len(record.RawSample)), header.Data_byte_len)
		t.Logf("header: %#v", *header)

		pos := uint32(unsafe.Sizeof(*header)) + uint32(header.Stack_byte_len)
		for pos < header.Data_byte_len {
			di := (*output.DataItemHeader)(unsafe.Pointer(&record.RawSample[pos]))
			typ, ok := irp.Types[ir.TypeID(di.Type)]
			if !ok {
				t.Fatalf("unknown type: %d", di.Type)
			}
			pos += uint32(unsafe.Sizeof(*di))
			t.Logf("di: %s @0x%x: %#v", typ.GetName(), di.Address, record.RawSample[pos:pos+uint32(di.Length)])
			pos += uint32(di.Length)
			if pos%8 > 0 {
				pos += 8 - pos%8
			}
		}
	}
}
