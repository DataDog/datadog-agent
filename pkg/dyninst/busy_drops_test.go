// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package dyninst_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uprobe"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestBusyDrops checks that a probe hit is dropped, rather than allowed to
// reuse the per-CPU scratch maps, when another invocation on the same CPU is
// in the middle of a run. It pins busyloop's threads to a single CPU and
// probes its tight loop, so that the scheduler switches between the threads
// while one of them is inside the probe. That only happens on kernels that
// preempt kernel code (preempt=full or preempt=lazy); on preempt=voluntary
// kernels the test only reports the count.
func TestBusyDrops(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	tasksetPath, err := exec.LookPath("taskset")
	if err != nil {
		t.Skip("taskset is not available")
	}
	var cfg testprogs.Config
	var found bool
	for _, c := range testprogs.MustGetCommonConfigs(t) {
		if c.GOARCH == runtime.GOARCH {
			cfg, found = c, true
			break
		}
	}
	if !found {
		t.Skipf("no test program built for %s", runtime.GOARCH)
	}
	busyloopPath := testprogs.MustGetBinary(t, "busyloop", cfg)
	mode := preemptMode(t)
	t.Logf("preemption mode: %q", mode)

	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-busy-drops-test-")
	defer cleanup()
	_, irp := dyninsttest.GenerateIr(
		t, tempDir, busyloopPath, "busyloop", irgen.WithSkipReturnEvents(true),
	)
	keepProbeWithID(&irp.Probes, "a")
	require.Equal(t, 1, len(irp.Probes))
	// Never throttle: a throttled hit returns before claiming the scratch
	// maps, so it could not be dropped.
	irp.Probes[0].ProbeDefinition = &overriddenThrottle{
		ProbeDefinition: irp.Probes[0].ProbeDefinition,
		periodMs:        1000 * 1000,
		budget:          math.MaxInt64 / 2,
	}
	program, cleanup := dyninsttest.CompileAndLoadBPF(t, tempDir, irp)
	defer cleanup()

	// Drain the ringbuf so that event submission keeps succeeding.
	rd, err := ringbuf.NewReader(program.Collection.Maps["out_ringbuf"])
	require.NoError(t, err)
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			if _, err := rd.Read(); err != nil {
				return
			}
		}
	}()
	defer func() {
		require.NoError(t, rd.Close())
		<-drained
	}()

	// taskset execs busyloop in place, so the PID stays the same and all of
	// busyloop's threads inherit the single-CPU affinity. The Go runtime
	// derives GOMAXPROCS from the affinity mask, so set it explicitly: with a
	// single P, the goroutines would take turns on one thread, and no other
	// thread would hit the probe while it is mid-run.
	const concurrency = 4
	t.Setenv("GOMAXPROCS", strconv.Itoa(concurrency))
	const roundSec = 5
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	proc, stdin := dyninsttest.StartProcess(
		ctx, t, tempDir, tasksetPath, "-c", "0", busyloopPath,
		"1" /* round_cnt */, strconv.Itoa(roundSec) /* round_sec */, strconv.Itoa(concurrency),
	)
	defer func() {
		_ = proc.Process.Kill()
		_ = proc.Wait()
	}()
	pid := process.ID{PID: int32(proc.Process.Pid)}
	wantExe, err := filepath.EvalSymlinks(busyloopPath)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid.PID))
		return err == nil && exe == wantExe
	}, 5*time.Second, 10*time.Millisecond, "taskset did not exec busyloop")
	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), pid.PID)
	require.NoError(t, err)
	attached, err := uprobe.Attach(program, exe, pid)
	require.NoError(t, err)
	defer func() { require.NoError(t, attached.Detach(nil)) }()

	_, err = stdin.Write([]byte("\n"))
	require.NoError(t, err)
	require.NoError(t, proc.Wait())

	var stats loader.RuntimeStats
	for _, s := range program.RuntimeStats() {
		stats.HitCnt += s.HitCnt
		stats.ThrottledCnt += s.ThrottledCnt
		stats.BusyDrops += s.BusyDrops
	}
	t.Logf(
		"hits: %d, throttled: %d, busy drops: %d",
		stats.HitCnt, stats.ThrottledCnt, stats.BusyDrops,
	)
	require.Greater(t, stats.HitCnt, uint64(0))
	require.Less(t, stats.BusyDrops, stats.HitCnt)
	switch mode {
	case "full", "lazy":
		require.Greater(t, stats.BusyDrops, uint64(0),
			"expected busy drops with preempt=%s", mode)
	default:
		t.Logf("not asserting on busy drops with preemption mode %q", mode)
	}
}

var currentPreemptModeRe = regexp.MustCompile(`\((\w+)\)`)

// preemptMode returns the kernel's current dynamic preemption mode, the
// entry shown in parentheses in /sys/kernel/debug/sched/preempt (e.g.
// "voluntary" or "lazy"), or "" if the file can't be read.
func preemptMode(t *testing.T) string {
	data, err := os.ReadFile("/sys/kernel/debug/sched/preempt")
	if err != nil {
		t.Logf("cannot read the preemption mode: %v", err)
		return ""
	}
	m := currentPreemptModeRe.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return string(m[1])
}
