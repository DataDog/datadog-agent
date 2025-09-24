// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"log"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestThrottler(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s", runtime.GOARCH)
			}
			bin := testprogs.MustGetBinary(t, "busyloop", cfg)
			t.Run("enforcesBudget", func(t *testing.T) {
				enforcesBudget(t, bin)
			})
			t.Run("refreshesBudget", func(t *testing.T) {
				refreshesBudget(t, bin)
			})
		})
	}
}

func enforcesBudget(t *testing.T, busyloopPath string) {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-throttler-test-")
	defer cleanup()

	// Load the binary and generate the IR.
	t.Logf("loading binary")
	obj, irp := dyninsttest.GenerateIr(t, tempDir, busyloopPath, "busyloop")

	// Adjust throttling parameters.
	// Practically infinite period, with specific event count.
	require.Equal(t, 1, len(irp.Probes))
	expectedEvents := 7

	irp.Probes[0].ProbeDefinition = &overriddenThrottle{
		ProbeDefinition: irp.Probes[0].ProbeDefinition,
		periodMs:        1000 * 1000,
		budget:          int64(expectedEvents),
	}

	// Compile the IR and prepare the BPF program.
	t.Logf("loading BPF")
	program, cleanup := dyninsttest.CompileAndLoadBPF(t, tempDir, irp)
	defer cleanup()

	// Launch the sample service, inject the BPF program and collect the output.
	t.Logf("running and instrumenting sample")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, busyloopPath,
		"1" /*round_cnt*/, "20" /*round_sec*/, "3", /*concurrency*/
	)
	cleanup = dyninsttest.AttachBPFProbes(
		t, busyloopPath, obj, sampleProc.Process.Pid, program,
	)
	defer cleanup()
	defer func() {
		sampleProc.Process.Kill()
		sampleProc.Process.Wait()
	}()
	sampleStdin.Write([]byte("\n"))

	// Read expected number of events.
	rd, err := ringbuf.NewReader(program.Collection.Maps["out_ringbuf"])
	require.NoError(t, err)
	rd.SetDeadline(time.Now().Add(10 * time.Second))
	for range expectedEvents {
		t.Logf("reading ringbuf item")
		_, err := rd.Read()
		require.NoError(t, err)
	}

	// Check there are no more events after a short delay.
	rd.SetDeadline(time.Now().Add(10 * time.Millisecond))
	v, err := rd.Read()
	log.Printf("err: %v", err)
	require.ErrorIs(t, err, os.ErrDeadlineExceeded, "expected deadline exceeded, got %#+v, %#+v", err, v)
}

func refreshesBudget(t *testing.T, busyloopPath string) {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-throttler-test-")
	defer cleanup()

	// Load the binary and generate the IR.
	t.Logf("loading binary")
	obj, irp := dyninsttest.GenerateIr(t, tempDir, busyloopPath, "busyloop")

	// Adjust throttling parameters.
	// Small period, and budget.
	require.Equal(t, 1, len(irp.Probes))
	irp.Probes[0].ProbeDefinition = &overriddenThrottle{
		ProbeDefinition: irp.Probes[0].ProbeDefinition,
		periodMs:        1,
		budget:          2,
	}

	// Compile the IR and prepare the BPF program.
	t.Logf("compiling BPF")
	program, cleanup := dyninsttest.CompileAndLoadBPF(t, tempDir, irp)
	defer cleanup()

	// Launch the sample service, inject the BPF program and collect the output.
	t.Logf("running and instrumenting sample")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, busyloopPath,
		"1" /*round_cnt*/, "20" /*round_sec*/, "3", /*concurrency*/
	)
	cleanup = dyninsttest.AttachBPFProbes(t, busyloopPath, obj, sampleProc.Process.Pid, program)
	defer cleanup()
	defer func() {
		sampleProc.Process.Kill()
		sampleProc.Process.Wait()
	}()
	sampleStdin.Write([]byte("\n"))

	// We should be able to observe multiple events.
	rd, err := ringbuf.NewReader(program.Collection.Maps["out_ringbuf"])
	require.NoError(t, err)
	rd.SetDeadline(time.Now().Add(10 * time.Second))
	for range 12 {
		t.Logf("reading ringbuf item")
		_, err := rd.Read()
		require.NoError(t, err)
	}
}

type overriddenThrottle struct {
	ir.ProbeDefinition
	periodMs uint32
	budget   int64
}

func (o *overriddenThrottle) GetThrottleConfig() ir.ThrottleConfig {
	return o
}

func (o *overriddenThrottle) GetThrottlePeriodMs() uint32 {
	return o.periodMs
}

func (o *overriddenThrottle) GetThrottleBudget() int64 {
	return o.budget
}
