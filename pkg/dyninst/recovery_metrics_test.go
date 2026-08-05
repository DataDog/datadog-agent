// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uprobe"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestRecoveryProbeMetrics exercises the panic_recover testprog and asserts
// that the BPF runtime.recovery handler bumps its per-CPU counters
// appropriately. The asserted invariants are:
//
//   - RecoveryFires equals the number of times runtime.recovery was called
//     (5 in the testprog: a→b→c, d→e→f, probedRecoverer→...→probedDeep,
//     goroutinePanic, and the unprobed-panicker goroutine).
//   - RecoveryEvictedFrames >= 5 — at least b, c, e, probedDeep, and
//     goroutineInner slots got evicted by the recovery handler.
//   - RecoveryNoOpenCalls >= 1 — the unprobedPanicker goroutine
//     triggers a recovery on a goroutine with no probed frames in
//     flight, exercising the short-circuit path.
//   - RecoveryFilteredGoexit = 0 — no Goexit unwinds in the testprog.
//   - RecoveryInvalidState = 0 — defensive bailout should not fire under
//     normal operation.
//   - RecoverySubmitFailures = 0 — the ringbuf is large enough not to
//     drop synthetic events in this small testprog.
func TestRecoveryProbeMetrics(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s",
					runtime.GOARCH)
			}
			bin := testprogs.MustGetBinary(t, "panic_recover", cfg)
			runRecoveryMetrics(t, bin)
		})
	}
}

func runRecoveryMetrics(t *testing.T, binPath string) {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-recovery-metrics-")
	defer cleanup()

	_, irp := dyninsttest.GenerateIr(t, tempDir, binPath, "panic_recover")

	program, cleanupBPF := dyninsttest.CompileAndLoadBPF(t, tempDir, irp)
	defer cleanupBPF()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, binPath,
	)
	defer func() {
		_ = sampleProc.Process.Kill()
		_ = sampleProc.Wait()
	}()

	pid := process.ID{PID: int32(sampleProc.Process.Pid)}
	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), pid.PID)
	require.NoError(t, err)
	attached, err := uprobe.Attach(program, exe, pid)
	require.NoError(t, err)
	defer func() { require.NoError(t, attached.Detach(nil)) }()

	// Drain the ringbuf in the background so the BPF side doesn't block
	// on a full ringbuf — that would produce submit failures which we
	// don't want for this test.
	rd, err := ringbuf.NewReader(program.Collection.Maps["out_ringbuf"])
	require.NoError(t, err)
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		rd.SetDeadline(time.Now().Add(20 * time.Second))
		for {
			if _, err := rd.Read(); err != nil {
				return
			}
		}
	}()

	_, _ = sampleStdin.Write([]byte("\n"))
	require.NoError(t, sampleProc.Wait())
	_ = rd.Close()
	<-drainDone

	// Aggregate per-CPU counters.
	var total loader.RuntimeStats
	for _, coreStats := range program.RuntimeStats() {
		total.RecoveryFires += coreStats.RecoveryFires
		total.RecoveryEvictedFrames += coreStats.RecoveryEvictedFrames
		total.RecoverySubmitFailures += coreStats.RecoverySubmitFailures
		total.RecoveryNoOpenCalls += coreStats.RecoveryNoOpenCalls
		total.RecoveryFilteredGoexit += coreStats.RecoveryFilteredGoexit
		total.RecoveryInvalidState += coreStats.RecoveryInvalidState
	}

	t.Logf("recovery stats: %+v", total)

	// The testprog deliberately triggers 5 recovery() calls. Allow >= 5
	// in case the runtime triggers internal panic+recover cycles we
	// haven't accounted for.
	assert.GreaterOrEqual(t, total.RecoveryFires, uint64(5),
		"recovery probe should have fired at least 5 times")

	// Probed frames that get evicted: b, c (scenario 1); e (scenario 3,
	// f has no pairing because it always panics); probedDeep (scenario
	// 4); goroutineInner (scenario 5). That's 5 evictions; scenario 6
	// doesn't probe anything inside the goroutine.
	assert.GreaterOrEqual(t, total.RecoveryEvictedFrames, uint64(5),
		"recovery probe should have evicted at least 5 frames")

	// Scenario 6: unprobedPanicker runs in a goroutine with no probed
	// frames; the recovery short-circuits on the in_progress_calls
	// lookup.
	assert.GreaterOrEqual(t, total.RecoveryNoOpenCalls, uint64(1),
		"recovery should have short-circuited on a no-open-calls goroutine")

	// No internal-error counters should be set under normal operation.
	assert.Equal(t, uint64(0), total.RecoveryFilteredGoexit,
		"no Goexit scenarios in this testprog")
	assert.Equal(t, uint64(0), total.RecoveryInvalidState,
		"no defensive bailouts expected")
	assert.Equal(t, uint64(0), total.RecoverySubmitFailures,
		"ringbuf should not be saturated by this small testprog")
}
