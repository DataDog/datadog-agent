// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"github.com/alecthomas/assert/v2"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
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
			bin := testprogs.GetBinary(t, "sample", cfg)
			testDyninst(t, bin)
		})
	}
}

func testDyninst(t *testing.T, sampleServicePath string) {
	t.Logf("loading binary")
	tempDir, err := os.MkdirTemp(os.TempDir(), "dyninst-integration-test-")
	require.NoError(t, err)
	defer func() {
		preserve, _ := strconv.ParseBool(os.Getenv("KEEP_TEMP"))
		if preserve || t.Failed() {
			t.Logf("leaving temp dir %s for inspection", tempDir)
		} else {
			require.NoError(t, os.RemoveAll(tempDir))
		}
	}()

	// Load the binary and generate the IR.
	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

	probes := testprogs.GetProbeCfgs(t, "sample")

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

	compiledBPF, err := compiler.CompileBPFProgram(irp, codeDump)
	require.NoError(t, err)
	defer func() { compiledBPF.Obj.Close() }()

	bpfObjDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)
	defer func() { require.NoError(t, bpfObjDump.Close()) }()
	_, err = io.Copy(bpfObjDump, compiledBPF.Obj)
	require.NoError(t, err)

	spec, err := ebpf.LoadCollectionSpecFromReader(compiledBPF.Obj)
	require.NoError(t, err)

	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{})
	require.NoError(t, err)
	defer func() { bpfCollection.Close() }()

	bpfProg, ok := bpfCollection.Programs[compiledBPF.ProgramName]
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

	textSection, err := obj.TextSectionHeader()
	require.NoError(t, err)
	var allAttached []link.Link
	for _, attachpoint := range compiledBPF.Attachpoints {
		// Despite the name, Uprobe expects an offset in the object file, and not the virtual address.
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		t.Logf("attaching @0x%x cookie=%d", addr, attachpoint.Cookie)
		attached, err := sampleLink.Uprobe(
			"",
			bpfProg,
			&link.UprobeOptions{
				PID:     sampleProc.Process.Pid,
				Address: addr,
				Offset:  0,
				Cookie:  attachpoint.Cookie,
			},
		)
		require.NoError(t, err)
		allAttached = append(allAttached, attached)
	}
	defer func() {
		for _, a := range allAttached {
			require.NoError(t, a.Close())
		}
	}()

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

	// Inlined function is called twice, hence extra event.
	for i := range len(probes) + 1 {
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

		// Decode the data with the corresponding IR used to generate it
		b := []byte{}
		out := bytes.NewBuffer(b)
		decoder, err := decode.NewDecoder(irp)
		assert.NoError(t, err)
		require.NotNil(t, decoder)

		err = decoder.Decode(record.RawSample, out)
		if err != nil {
			t.Logf("error decoding: %s", err)
		}
		t.Logf("Decoded output for probe %s:\n %s", probes[i].GetID(), out.String())
	}
}
