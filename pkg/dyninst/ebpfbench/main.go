// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Code skeleton for running a manual benchmark of eBPF program cpu overheads.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func getSampleBinaryPath() (string, error) {
	return testprogs.GetBinary("busyloop", testprogs.Config{
		GOARCH:      runtime.GOARCH,
		GOTOOLCHAIN: "go1.24.3",
	})
}

func runBenchmark() error {
	binPath, err := getSampleBinaryPath()
	if err != nil {
		return err
	}

	fmt.Printf("loading binary %s\n", binPath)
	// Load the binary and generate the IR.
	binary, err := safeelf.Open(binPath)
	if err != nil {
		return err
	}
	defer func() { binary.Close() }()

	probes, err := testprogs.GetProbeDefinitions("busyloop")
	if err != nil {
		return err
	}

	obj, err := object.NewElfObject(binary)
	if err != nil {
		return err
	}

	irp, err := irgen.GenerateIR(1, obj, probes)
	if err != nil {
		return err
	}
	irp.Probes[0].ThrottlePeriodMs = 1
	irp.Probes[0].ThrottleBudget = 3

	// Compile the IR and prepare the BPF program.
	fmt.Println("compiling BPF")
	compiledBPF, err := compiler.NewCompiler().Compile(irp, nil)
	if err != nil {
		return err
	}
	defer func() { compiledBPF.Obj.Close() }()

	bpfObjDump, err := os.Create("/tmp/probe.bpf.o")
	if err != nil {
		return err
	}
	defer func() { bpfObjDump.Close() }()
	_, err = io.Copy(bpfObjDump, compiledBPF.Obj)
	if err != nil {
		return err
	}

	fmt.Println("loading BPF")
	spec, err := ebpf.LoadCollectionSpecFromReader(compiledBPF.Obj)
	if err != nil {
		return err
	}

	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{})
	if err != nil {
		return err
	}
	defer func() { bpfCollection.Close() }()

	bpfProg, ok := bpfCollection.Programs[compiledBPF.ProgramName]
	if !ok {
		return fmt.Errorf("bpf program %s not found", compiledBPF.ProgramName)
	}

	sampleLink, err := link.OpenExecutable(binPath)
	if err != nil {
		return err
	}

	// Launch the sample service, inject the BPF program and collect the output.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	fmt.Println("spawning and instrumenting sample")
	sampleProc := exec.CommandContext(ctx, binPath, "5", "1", "4")
	sampleStdin, err := sampleProc.StdinPipe()
	if err != nil {
		return err
	}
	sampleProc.Stdout = os.Stdout
	sampleProc.Stderr = os.Stderr
	err = sampleProc.Start()
	if err != nil {
		return err
	}

	textSection, err := object.FindTextSectionHeader(obj.File)
	if err != nil {
		return err
	}
	var allAttached []link.Link
	for _, attachpoint := range compiledBPF.Attachpoints {
		// Despite the name, Uprobe expects an offset in the object file, and not the virtual address.
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		fmt.Printf("attaching @0x%x cookie=%d\n", addr, attachpoint.Cookie)
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
		if err != nil {
			return err
		}
		allAttached = append(allAttached, attached)
	}
	defer func() {
		for _, a := range allAttached {
			if err := a.Close(); err != nil {
				fmt.Printf("error closing link: %v\n", err)
			}
		}
	}()

	// Trigger the function calls.
	fmt.Println("running the sample")
	_, err = sampleStdin.Write([]byte("\n"))
	if err != nil {
		return err
	}

	err = sampleProc.Wait()
	if err != nil {
		return err
	}

	// Count events.
	rd, err := ringbuf.NewReader(bpfCollection.Maps["out_ringbuf"])
	if err != nil {
		return err
	}

	ts := make(map[uint64]struct{})
	duplicates := 0

	events := 0
	for rd.AvailableBytes() > 0 {
		record, err := rd.Read()
		if err != nil {
			return err
		}

		header := (*output.EventHeader)(unsafe.Pointer(&record.RawSample[0]))
		_, ok := ts[header.Ktime_ns]
		if ok {
			duplicates++
		}
		ts[header.Ktime_ns] = struct{}{}
		events++
	}
	fmt.Printf("collected %d events with %d ts duplicates\n", events, duplicates)

	return nil
}

func main() {
	err := runBenchmark()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
