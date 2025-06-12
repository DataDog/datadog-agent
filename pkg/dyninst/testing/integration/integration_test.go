// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// This test is used to run the dynamic instrumentation product on a sample binary from end to end.
// Update test data by setting REWRITE=true and running the test.
//
// You can run individual probes for a given architecture and toolchain like so:
// go test -run TestDyninst/test_single_int32_arch=arm64,toolchain=go1.24.3
//
// You can run all probes at once for a given architecture and toolchain like so:
// go test -run TestDyninst/test_single_int32_arch=arm64,toolchain=go1.24.3/all

package testprogs

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

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	object "github.com/DataDog/datadog-agent/pkg/dyninst/object"
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

var tempDir string

func TestDyninst(t *testing.T) {
	skipIfKernelNotSupported(t)
	cfgs := GetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s", runtime.GOARCH)
			}
			bin := GetBinary(t, "sample", cfg)
			var err error
			tempDir, err = os.MkdirTemp(os.TempDir(), "dyninst-integration-test-")
			require.NoError(t, err)
			defer func() {
				preserve, _ := strconv.ParseBool(os.Getenv("KEEP_TEMP"))
				if preserve || t.Failed() {
					t.Logf("leaving temp dir %s for inspection", tempDir)
				} else {
					require.NoError(t, os.RemoveAll(tempDir))
				}
			}()

			probes := GetProbeCfgs(t, "sample")
			expectedOutput := GetExpectedOutput(t, "sample")
			for i := range probes {
				t.Run(probes[i].GetID(), func(t *testing.T) {
					testSingleProbe(t, bin, probes[i], expectedOutput)
				})
			}

			t.Run("all", func(t *testing.T) {
				testAllProbesAtOnce(t, bin, probes, expectedOutput)
			})
		})
	}
}

func testSingleProbe(t *testing.T, sampleServicePath string, probe config.Probe, expectedOutput map[string]string) {

	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)

	t.Logf("Generating IR for probe %s\n", probe.GetID())
	irp, err := irgen.GenerateIR(1, obj, []config.Probe{probe})
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

	t.Logf("reading ringbuf item")
	require.Greater(t, rd.AvailableBytes(), 0)
	record, err := rd.Read()
	require.NoError(t, err)

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

	assert.Equal(t, expectedOutput["main."+probe.GetID()], out.String())

	// Save actual output to YAML file if REWRITE is set
	if saveOutput, _ := strconv.ParseBool(os.Getenv("REWRITE")); saveOutput {
		expectedOutput["main."+probe.GetID()] = out.String()
		SaveActualOutput(t, "sample", expectedOutput)
	}
}

func testAllProbesAtOnce(t *testing.T, sampleServicePath string, probes []config.Probe, expectedOutput map[string]string) {
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

	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

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

	// Have a 'reverse map' of probe output, to ensure all events are received
	// and no unexpected events are received.
	probeOutput := map[string]struct{}{}
	for _, o := range expectedOutput {
		probeOutput[o] = struct{}{}
	}

	for range len(probes) + 1 {
		t.Logf("reading ringbuf item")
		require.Greater(t, rd.AvailableBytes(), 0)
		record, err := rd.Read()
		require.NoError(t, err)

		b := []byte{}
		out := bytes.NewBuffer(b)
		decoder, err := decode.NewDecoder(irp)
		assert.NoError(t, err)
		require.NotNil(t, decoder)

		err = decoder.Decode(record.RawSample, out)
		if err != nil {
			t.Errorf("error decoding: %s", err)
		}

		_, ok := probeOutput[out.String()]
		if !ok {
			t.Errorf("unexpected output: %s", out.String())
		}
	}
}
