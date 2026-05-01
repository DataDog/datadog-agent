// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/eventbuf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestFirstFlushFailDropNotification deterministically exercises the
// "first flush fails" case in the BPF stack machine: a probe invocation
// needs continuation, but its very first scratch_buf_flush_and_continue
// call hits a saturated output ringbuf and fails before any fragment
// reaches userspace.
//
// Two variants:
//
//   - Variant A (capture_chain_return, return-side): tiny entry chain
//     (one node) — the entry's single capture record is small enough
//     to submit cleanly into a near-saturated ringbuf. Large return
//     chain (1000 nodes) — needs continuation; its first flush hits
//     the saturated ringbuf with no return fragments yet in flight.
//
//   - Variant B (capture_chain, entry-side): a large entry chain that
//     needs continuation directly; its first flush hits the saturated
//     ringbuf with no fragments yet in flight.
//
// Saturation strategy: install a controlledSink that wraps the
// production sink and blocks HandleEvent on demand. While blocking,
// the dispatcher's output-reader goroutine stalls and the output
// ringbuf fills up. The drop-notify reader runs on its own goroutine
// and is never held, so the test still observes drop notifications.
// We saturate to a target free-space window (16 KiB free in a 64 KiB
// ringbuf) — well below SCRATCH_BUF_LEN (32 KiB) so the trigger's
// first 32 KiB flush is guaranteed to fail, but well above the
// trigger's small entry record size in variant A so the entry can
// still submit.
//
// The bug being verified is in event.c's continuation_aborted branch:
// when last_submitted_seq is still LAST_SUBMITTED_SEQ_NONE (0xFFFF) at
// the moment the first flush fails, pre-fix BPF emits PARTIAL_*
// carrying that sentinel as last_seq. Userspace's NotePartial then
// computes entryExpected = lastSeq + 1 with uint16, which wraps to 0
// — the eventbuf semantics treat that as "still assembling, total
// unknown" and the entry sits stranded indefinitely. The fix gates
// PARTIAL_* on a non-sentinel last_submitted_seq and falls through to
// RETURN_LOST for return-side / suppress for entry-side when no
// fragments were submitted.
//
// Diagnostic assertions, evaluated after sink.stopBlocking() and a
// drain window:
//
//  1. No observed drop notification carries the sentinel last_seq=0xFFFF.
//     This is the precise contract the BPF fix establishes; pre-fix the
//     test fails because both variants observe such notifications.
//
//  2. Variant A: at least one snapshot for the capture_chain_return
//     probe ID lands in fakeintake. With the fix, the entry submits
//     cleanly and is finalized via the RETURN_LOST notification; pre-fix
//     the entry is stranded with entryExpected=0 and no snapshot lands.
//
//  3. Both variants (recovery): after drain, a fresh non-saturating
//     trigger invocation produces a snapshot. This confirms the system
//     is not poisoned — neither the eventbuf nor the BPF state machine
//     is left in a state that prevents future events.
func TestFirstFlushFailDropNotification(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	t.Parallel()

	cfgs := testprogs.MustGetCommonConfigs(t)
	var cfg testprogs.Config
	var found bool
	for _, c := range cfgs {
		if c.GOARCH == runtime.GOARCH {
			cfg = c
			found = true
			break
		}
	}
	if !found {
		t.Skipf("no drop_tester config matches runtime arch %s", runtime.GOARCH)
	}
	t.Logf("using config %s", cfg.String())

	// Collect trace_pipe output so BPF debug logs (`LOG(N, ...)` calls
	// in event.c, scratch.h, stack_machine.h) are available for
	// post-mortem on failure. The collector organizes output by PID;
	// each variant gets its own subject process. Set
	// DONT_COLLECT_TRACE_PIPE=1 to disable when running outside CI
	// (the readLoop holds an open trace_pipe fd which is sometimes
	// inconvenient).
	var collector *tracePipeCollector
	if dontCollect, _ := strconv.ParseBool(os.Getenv("DONT_COLLECT_TRACE_PIPE")); !dontCollect {
		collector = newTracePipeCollector(t)
		t.Cleanup(func() { collector.Close() })
	}

	t.Run("variant_A_return_side_first_flush_fails", func(t *testing.T) {
		runFirstFlushFailScenario(t, cfg, collector, firstFlushFailVariant{
			name: "variant_A",
			// Tiny entry chain (one node) + large return chain. Entry
			// capture is small (single-node deref ≈ 80 bytes data + ~88
			// header < 200 bytes ringbuf record), submits cleanly into
			// the 16 KiB free window. Return capture chases 1000 nodes,
			// produces multi-fragment payload; first 32 KiB flush fails
			// against the saturated ringbuf.
			triggerPath: "/chain_return?iter=1&entry_nodes=1&return_nodes=1000",
			// Post-fix: the entry submits, return emits RETURN_LOST(0),
			// eventbuf finalizes the entry alone. Snapshot lands.
			expectTriggerSnapshot: true,
			triggerProbeID:        "capture_chain_return",
		})
	})

	t.Run("variant_B_entry_side_first_flush_fails", func(t *testing.T) {
		runFirstFlushFailScenario(t, cfg, collector, firstFlushFailVariant{
			name: "variant_B",
			// Large entry chain. First flush attempts a 32 KiB submit
			// against the 16 KiB free window — fails. No fragments in
			// flight when continuation_aborted triggers.
			triggerPath: "/chain?iter=1&size=1000",
			// Post-fix: BPF suppresses the notification (entry-side, no
			// fragments). No userspace state for this invocation; no
			// snapshot lands. Pre-fix: BPF emits PARTIAL_ENTRY(0xFFFF),
			// userspace strands the entry — also no snapshot, but for
			// the wrong reason. The bug is detected by the
			// no-sentinel-last_seq assertion.
			expectTriggerSnapshot: false,
			triggerProbeID:        "capture_chain",
		})
	})
}

type firstFlushFailVariant struct {
	name                  string
	triggerPath           string
	expectTriggerSnapshot bool
	triggerProbeID        string
}

// controlledSink wraps a real dispatcher.Sink and, when blocking is
// enabled, holds HandleEvent calls so the dispatcher's output-reader
// goroutine stalls and the output ringbuf saturates. HandleDropNotification
// runs on a separate goroutine in the dispatcher and is never held
// here, so the test can still observe drop notifications while events
// are blocked.
type controlledSink struct {
	real   dispatcher.Sink
	buffer *eventbuf.Buffer
	budget *eventbuf.Budget

	blocking atomic.Bool
	release  chan struct{}
	drops    chan output.DropNotification
}

func newControlledSink(real dispatcher.Sink, buffer *eventbuf.Buffer, budget *eventbuf.Budget) *controlledSink {
	return &controlledSink{
		real:    real,
		buffer:  buffer,
		budget:  budget,
		release: make(chan struct{}),
		drops:   make(chan output.DropNotification, 256),
	}
}

func (c *controlledSink) HandleEvent(m dispatcher.Message) error {
	for c.blocking.Load() {
		<-c.release
	}
	return c.real.HandleEvent(m)
}

func (c *controlledSink) HandleDropNotification(d output.DropNotification) {
	select {
	case c.drops <- d:
	default:
		// Buffer is sized for the test workload; drops past capacity
		// would indicate either a misconfigured test or a regression
		// in the BPF side. The sentinel-last_seq assertion below will
		// surface the discrepancy if a real PARTIAL_*(0xFFFF) was
		// dropped.
	}
	c.real.HandleDropNotification(d)
}

func (c *controlledSink) EvictOlderThan(t uint64) { c.real.EvictOlderThan(t) }

func (c *controlledSink) Close() {
	// Ensure no goroutine is parked on c.release when the dispatcher
	// shuts us down.
	c.blocking.Store(false)
	select {
	case <-c.release:
	default:
		close(c.release)
	}
	c.real.Close()
}

func (c *controlledSink) startBlocking() { c.blocking.Store(true) }

func (c *controlledSink) stopBlocking() {
	c.blocking.Store(false)
	select {
	case <-c.release:
	default:
		close(c.release)
	}
}

// ringbufBytes is the per-test output-ringbuf size. 64 KiB is the
// smallest power-of-two that admits a single SCRATCH_BUF_LEN (32 KiB)
// fragment with headroom. Smaller would prevent any fragment from
// ever submitting; larger would just need more filler invocations to
// saturate.
const ringbufBytes = 64 << 10

func runFirstFlushFailScenario(
	t *testing.T,
	cfg testprogs.Config,
	collector *tracePipeCollector,
	variant firstFlushFailVariant,
) {
	t.Parallel()
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-first-flush-fail-"+variant.name)
	defer cleanup()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	loaderOpts := []loader.Option{
		loader.WithRingBufSize(ringbufBytes),
	}
	// Enable BPF debug logging so trace_pipe captures LOG(N, ...) lines
	// from event.c (including the "probe_run: continuation aborted at
	// seq=%d" line we want to verify is firing).
	loaderOpts = append(loaderOpts, loader.WithDebugLevel(100))
	modCfg.TestingKnobs.LoaderOptions = loaderOpts
	modCfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	modCfg.LogUploaderURL = testServer.getLogsURL()
	modCfg.DiagsUploaderURL = testServer.getDiagsURL()

	var sendUpdate fakeProcessSubscriber
	modCfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close()
		return &sendUpdate
	}
	modCfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	modCfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &backoff.ExpBackoffPolicy{
			MaxBackoffTime: time.Millisecond.Seconds(),
		},
	}
	modCfg.ActuatorConfig.RecompilationRateLimit = -1

	var (
		mu   sync.Mutex
		sink *controlledSink
		prog *loader.Program
		ldr  *loader.Loader
	)
	modCfg.TestingKnobs.SinkOverride = func(
		real dispatcher.Sink,
		buf *eventbuf.Buffer,
		budget *eventbuf.Budget,
	) dispatcher.Sink {
		s := newControlledSink(real, buf, budget)
		mu.Lock()
		sink = s
		mu.Unlock()
		return s
	}
	modCfg.TestingKnobs.OnLoaderReady = func(l *loader.Loader) {
		mu.Lock()
		ldr = l
		mu.Unlock()
	}
	modCfg.TestingKnobs.OnProgramLoaded = func(p *loader.Program) {
		mu.Lock()
		prog = p
		mu.Unlock()
	}

	m, err := module.NewModule(modCfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	binPath := testprogs.MustGetBinary(t, "drop_tester", cfg)
	require.NotEmpty(t, binPath,
		"drop_tester binary not built; run `dda inv system-probe.build-dyninst-test-programs`")

	ctx := context.Background()
	proc, _ := dyninsttest.StartProcess(ctx, t, tempDir, binPath)
	pid := proc.Process.Pid
	t.Logf("launched drop_tester with pid %d", pid)
	defer func() {
		_ = proc.Process.Signal(os.Interrupt)
		_ = proc.Wait()
	}()

	// On failure (or when FORCE_TRACE_PIPE_PRINT=1) dump the BPF
	// trace_pipe output for this PID so the verifier-level reasoning
	// is visible. Particularly useful for confirming that LOG(1)
	// lines like "probe_run: continuation aborted at seq=%d" actually
	// fire — i.e., the test really did hit the buggy code path.
	forceTracePipePrint, _ := strconv.ParseBool(os.Getenv("FORCE_TRACE_PIPE_PRINT"))
	t.Cleanup(func() {
		if collector == nil || (!t.Failed() && !forceTracePipePrint) {
			return
		}
		if err := collector.Flush(); err != nil {
			t.Logf("trace pipe flush error: %v", err)
		}
		f, err := collector.GetLogs(pid)
		if err != nil {
			t.Logf("trace pipe GetLogs error: %v", err)
			return
		}
		if f == nil {
			return
		}
		defer f.Close()
		t.Logf("--- trace_pipe output for pid %d ---", pid)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			t.Log(scanner.Text())
		}
		t.Logf("--- end trace_pipe output ---")
	})

	port := waitForDropTesterPort(t, filepath.Join(tempDir, "sample.out"))
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	probes := testprogs.MustGetProbeDefinitions(t, "drop_tester")
	require.NotEmpty(t, probes, "no probes registered for drop_tester")
	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(pid))
	require.NoError(t, err)
	sendUpdate(process.ProcessesUpdate{
		Updates: []process.Config{
			{
				Info: process.Info{
					ProcessID:   process.ID{PID: int32(pid)},
					Executable:  exe,
					Service:     "drop_tester",
					ProcessTags: []string{"entrypoint.name:drop_tester"},
				},
				RuntimeID: "drop_tester_test",
				Probes:    slices.Clone(probes),
			},
		},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		installed := map[string]struct{}{}
		for _, d := range testServer.getDiags() {
			if d.diagnosticMessage.Debugger.Status == uploader.StatusInstalled {
				installed[d.diagnosticMessage.Debugger.ProbeID] = struct{}{}
			} else if d.diagnosticMessage.Debugger.Status == uploader.StatusError {
				t.Fatalf("probe %s install error: %s",
					d.diagnosticMessage.Debugger.ProbeID,
					d.diagnosticMessage.Debugger.DiagnosticException.Message)
			}
		}
		want := map[string]struct{}{}
		for _, p := range probes {
			want[p.GetID()] = struct{}{}
		}
		assert.Equal(c, want, installed)
	}, 60*time.Second, 100*time.Millisecond,
		"probes should install within 60s")

	mu.Lock()
	require.NotNil(t, sink, "controlledSink was not installed")
	require.NotNil(t, prog, "loader.Program handle was not captured")
	require.NotNil(t, ldr, "loader.Loader handle was not captured")
	mu.Unlock()

	// Make sure the sink is unparked before module shutdown runs in
	// cleanup. Without this, a t.Fatalf mid-test would leave the sink
	// blocking and the dispatcher's flushAndWait would hang forever
	// waiting for the run loop to drain. Registered before
	// startBlocking so cleanup (LIFO) unparks before anything else.
	t.Cleanup(sink.stopBlocking)

	client := &http.Client{Timeout: 30 * time.Second}
	doGet := func(path string) {
		t.Helper()
		resp, err := client.Get(baseURL + path)
		require.NoError(t, err, "GET %s", path)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", path)
	}

	// Step 1: block the dispatcher's output reader.
	sink.startBlocking()

	// Step 2: saturate the output ringbuf to the target free-space
	// window. We send filler invocations one batch at a time and poll
	// AvailableBytes() between batches. Each filler probe firing
	// produces one ringbuf record of ~600 bytes; we batch enough to
	// converge quickly without overshooting past zero free.
	saturateRingbuf(t, ldr, doGet)

	// Step 3: issue the trigger request. The probe invocation will
	// need continuation; its first flush will fail because free space
	// is < SCRATCH_BUF_LEN.
	doGet(variant.triggerPath)

	// Step 4: collect drop notifications for a fixed window. We don't
	// gate on the first arrival because some variants expect zero
	// trigger-side notifications post-fix (entry-side, no fragments).
	var observedDrops []output.DropNotification
	collectTimer := time.NewTimer(2 * time.Second)
collect:
	for {
		select {
		case d := <-sink.drops:
			observedDrops = append(observedDrops, d)
		case <-collectTimer.C:
			break collect
		}
	}

	// Step 5: release the sink and drain.
	sink.stopBlocking()
	time.Sleep(2 * time.Second)

	for i, d := range observedDrops {
		t.Logf("observed drop notification[%d]: reason=%d last_seq=%d probe_id=%d goid=%d entry_ktime_ns=%d",
			i, d.Drop_reason, d.Last_seq, d.Probe_id, d.Goid, d.Entry_ktime_ns)
	}

	// Assertion 1: no drop notification carries the sentinel last_seq.
	// This is the precise contract the BPF fix establishes.
	for i, d := range observedDrops {
		assert.NotEqual(t, uint16(0xFFFF), d.Last_seq,
			"drop notification[%d] must not carry sentinel last_seq=0xFFFF (reason=%d, probe_id=%d)",
			i, d.Drop_reason, d.Probe_id)
	}

	// Sanity: the side channel must not have lost notifications during
	// the test, otherwise assertion 1 is unreliable.
	assert.Equal(t, uint64(0), prog.DropNotifyLostAt(),
		"drop-notify side channel must not have lost notifications during the test")

	// Snapshot the logs collected so far (before the recovery probe).
	logsAfterDrain := testServer.getLogs()
	t.Logf("after drain: %d snapshot logs", len(logsAfterDrain))
	t.Logf("end-of-test eventbuf state: Len=%d Bytes=%d budget.Used=%d",
		sink.buffer.Len(), sink.buffer.Bytes(), sink.budget.Used())

	// Assertion 2 (variant-specific): trigger snapshot landed iff
	// expected. We identify "the trigger's snapshot" by probe ID —
	// the trigger is the only invocation of variant.triggerProbeID;
	// fillers and recovery probes use different probes.
	triggerSnapshots := countSnapshotsForProbe(t, logsAfterDrain, variant.triggerProbeID)
	if variant.expectTriggerSnapshot {
		assert.Greater(t, triggerSnapshots, 0,
			"expected at least one snapshot for trigger probe %s "+
				"(post-fix: entry submits cleanly and is finalized via RETURN_LOST)",
			variant.triggerProbeID)
	} else {
		assert.Equal(t, 0, triggerSnapshots,
			"expected no snapshot for trigger probe %s "+
				"(post-fix: entry-side first-flush-fails emits no notification, no userspace state)",
			variant.triggerProbeID)
	}

	// Assertion 3 (recovery): a fresh non-saturating invocation after
	// the drain produces a snapshot. Verifies neither the eventbuf
	// nor the BPF state machine is left in a state that prevents
	// future events.
	doGet("/bytes?iter=1&size=128")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		recovery := len(testServer.getLogs()) - len(logsAfterDrain)
		assert.Greater(c, recovery, 0,
			"recovery: a fresh /bytes invocation must produce a snapshot after drain")
	}, 10*time.Second, 100*time.Millisecond)

	// Stop the subject process and drain.
	_ = proc.Process.Signal(os.Interrupt)
	_ = proc.Wait()
	sendUpdate(process.ProcessesUpdate{
		Removals: []process.ID{{PID: int32(pid)}},
	})
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Empty(c, m.DiagnosticsStates())
	}, 30*time.Second, 100*time.Millisecond,
		"diagnostics states should drain after process exit")

	t.Logf("collected %d snapshot logs total", len(testServer.getLogs()))
}

// saturateRingbuf sends filler /bytes invocations until
// OutputReader().AvailableBytes() is high enough that the next
// SCRATCH_BUF_LEN-sized flush is guaranteed to fail (free space <
// 32 KiB), but low enough that the variant-A entry record (a few
// hundred bytes) can still submit cleanly.
//
// The acceptable free-space window is conservative on both ends:
//
//   - Upper bound: free <= 32 KiB - 8 (so the next 32 KiB flush is
//     guaranteed to fail, accounting for the 8-byte ringbuf record
//     header).
//
//   - Lower bound: free >= 1 KiB (variant A's tiny-chain entry record
//     is well under 1 KiB; submitting it must succeed).
//
// We approach the window from below: start with a few large batches
// to get close, then switch to single-invocation steps for fine
// control near the target. Aborts the test if the window can't be
// hit within a generous budget.
func saturateRingbuf(t *testing.T, ldr *loader.Loader, doGet func(string)) {
	t.Helper()
	const fillerSize = 512
	// freeMin: minimum free space that variant A's small entry record
	// can still submit into. Set well above the actual record size
	// (~200 bytes) for headroom.
	const freeMin = 1 << 10
	// freeMax: maximum free space such that the next 32 KiB flush is
	// guaranteed to fail. SCRATCH_BUF_LEN = 32 KiB, plus 8-byte ringbuf
	// header = 32776 needed. Anything < 32776 free triggers the fail.
	const freeMax = (32 << 10) - 8
	const targetUsedLo = ringbufBytes - freeMax // = 32776
	const targetUsedHi = ringbufBytes - freeMin // = 64512

	step := func(batchSize int) {
		doGet(fmt.Sprintf("/bytes?iter=%d&size=%d", batchSize, fillerSize))
		time.Sleep(50 * time.Millisecond)
	}

	maxBatches := 64
	for batch := 0; batch < maxBatches; batch++ {
		used := ldr.OutputReader().AvailableBytes()
		if used >= targetUsedLo && used <= targetUsedHi {
			t.Logf("ringbuf saturated to target window after %d steps (used=%d, target=[%d,%d], capacity=%d)",
				batch, used, targetUsedLo, targetUsedHi, ringbufBytes)
			return
		}
		if used > targetUsedHi {
			t.Fatalf("overshot ringbuf saturation: used=%d > %d "+
				"(if this fires, decrease step size near the target)",
				used, targetUsedHi)
		}
		// Pick batch size based on distance from the lower bound.
		// Far away: batch=16 to converge fast. Close: batch=1 for
		// fine control.
		remaining := targetUsedLo - used
		batchSize := 16
		if remaining < 8*1024 {
			batchSize = 4
		}
		if remaining < 2*1024 {
			batchSize = 1
		}
		step(batchSize)
	}
	t.Fatalf("failed to saturate ringbuf to target window within %d steps; final used=%d",
		maxBatches, ldr.OutputReader().AvailableBytes())
}

// TestFragmentCap exercises the per-invocation MAX_CONTINUATION_FRAGMENTS
// cap added in the previous commit. The cap bounds the raw bytes a
// single probe invocation can emit at MAX_CONTINUATION_FRAGMENTS *
// SCRATCH_BUF_LEN = 16 * 32 KiB = 512 KiB, sized to keep the JSON-
// serialized snapshot under the backend's 1 MiB ceiling even after
// ~2x binary-to-JSON inflation.
//
// The trigger is a chain of nodes each carrying a string. Strings flow
// through chased_slices (cap 128) instead of the chased-pointers trie
// (cap 1024), so the chain can produce arbitrarily many bytes without
// exhausting the trie. We pick 128 nodes × 4 KiB strings = ~530 KiB
// raw, comfortably above the 512 KiB cap. (A plain Node chain can't
// reach the cap because each Node consumes a trie slot.)
//
// This test uses a generously-sized output ringbuf (8 MiB — the
// production default) so saturation isn't a confound: any fragment
// truncation here is the cap, not ringbuf pressure.
//
// Diagnostic assertions:
//
//  1. Exactly one drop notification is observed for the trigger probe,
//     reason=PARTIAL_ENTRY, last_seq=MAX_CONTINUATION_FRAGMENTS-1=15.
//     This is the precise contract: 16 fragments [0..15] submitted,
//     no more accepted.
//
//  2. A snapshot for capture_string_chain lands in fakeintake (the
//     truncated event with 16 fragments).
//
//  3. After the trigger, a fresh smaller invocation produces a normal
//     snapshot (recovery — the BPF state machine isn't poisoned).
func TestFragmentCap(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	current := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, current) })

	cfgs := testprogs.MustGetCommonConfigs(t)
	var cfg testprogs.Config
	var found bool
	for _, c := range cfgs {
		if c.GOARCH == runtime.GOARCH {
			cfg = c
			found = true
			break
		}
	}
	if !found {
		t.Skipf("no drop_tester config matches runtime arch %s", runtime.GOARCH)
	}
	t.Logf("using config %s", cfg.String())

	var collector *tracePipeCollector
	if dontCollect, _ := strconv.ParseBool(os.Getenv("DONT_COLLECT_TRACE_PIPE")); !dontCollect {
		collector = newTracePipeCollector(t)
		t.Cleanup(func() { collector.Close() })
	}

	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-fragment-cap")
	defer cleanup()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	loaderOpts := []loader.Option{
		// Generously-sized ringbuf so saturation is not a confound;
		// any truncation here is the cap, not ringbuf pressure.
		loader.WithRingBufSize(8 << 20),
		loader.WithDebugLevel(100),
	}
	modCfg.TestingKnobs.LoaderOptions = loaderOpts
	modCfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	modCfg.LogUploaderURL = testServer.getLogsURL()
	modCfg.DiagsUploaderURL = testServer.getDiagsURL()

	var sendUpdate fakeProcessSubscriber
	modCfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close()
		return &sendUpdate
	}
	modCfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	modCfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &backoff.ExpBackoffPolicy{
			MaxBackoffTime: time.Millisecond.Seconds(),
		},
	}
	modCfg.ActuatorConfig.RecompilationRateLimit = -1

	// Wire up a controlledSink purely for drop-notification observability —
	// we do NOT block events here. The sink forwards everything to the
	// production sink (HandleEvent + HandleDropNotification).
	var (
		mu   sync.Mutex
		sink *controlledSink
		prog *loader.Program
	)
	modCfg.TestingKnobs.SinkOverride = func(
		real dispatcher.Sink,
		buf *eventbuf.Buffer,
		budget *eventbuf.Budget,
	) dispatcher.Sink {
		s := newControlledSink(real, buf, budget)
		mu.Lock()
		sink = s
		mu.Unlock()
		return s
	}
	modCfg.TestingKnobs.OnProgramLoaded = func(p *loader.Program) {
		mu.Lock()
		prog = p
		mu.Unlock()
	}

	m, err := module.NewModule(modCfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	binPath := testprogs.MustGetBinary(t, "drop_tester", cfg)
	require.NotEmpty(t, binPath,
		"drop_tester binary not built; run `dda inv system-probe.build-dyninst-test-programs`")

	ctx := context.Background()
	proc, _ := dyninsttest.StartProcess(ctx, t, tempDir, binPath)
	pid := proc.Process.Pid
	t.Logf("launched drop_tester with pid %d", pid)
	defer func() {
		_ = proc.Process.Signal(os.Interrupt)
		_ = proc.Wait()
	}()

	forceTracePipePrint, _ := strconv.ParseBool(os.Getenv("FORCE_TRACE_PIPE_PRINT"))
	t.Cleanup(func() {
		if collector == nil || (!t.Failed() && !forceTracePipePrint) {
			return
		}
		if err := collector.Flush(); err != nil {
			t.Logf("trace pipe flush error: %v", err)
		}
		f, err := collector.GetLogs(pid)
		if err != nil {
			t.Logf("trace pipe GetLogs error: %v", err)
			return
		}
		if f == nil {
			return
		}
		defer f.Close()
		t.Logf("--- trace_pipe output for pid %d ---", pid)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			t.Log(scanner.Text())
		}
		t.Logf("--- end trace_pipe output ---")
	})

	port := waitForDropTesterPort(t, filepath.Join(tempDir, "sample.out"))
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	probes := testprogs.MustGetProbeDefinitions(t, "drop_tester")
	require.NotEmpty(t, probes, "no probes registered for drop_tester")
	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(pid))
	require.NoError(t, err)
	sendUpdate(process.ProcessesUpdate{
		Updates: []process.Config{
			{
				Info: process.Info{
					ProcessID:   process.ID{PID: int32(pid)},
					Executable:  exe,
					Service:     "drop_tester",
					ProcessTags: []string{"entrypoint.name:drop_tester"},
				},
				RuntimeID: "drop_tester_test",
				Probes:    slices.Clone(probes),
			},
		},
	})

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		installed := map[string]struct{}{}
		for _, d := range testServer.getDiags() {
			if d.diagnosticMessage.Debugger.Status == uploader.StatusInstalled {
				installed[d.diagnosticMessage.Debugger.ProbeID] = struct{}{}
			} else if d.diagnosticMessage.Debugger.Status == uploader.StatusError {
				t.Fatalf("probe %s install error: %s",
					d.diagnosticMessage.Debugger.ProbeID,
					d.diagnosticMessage.Debugger.DiagnosticException.Message)
			}
		}
		want := map[string]struct{}{}
		for _, p := range probes {
			want[p.GetID()] = struct{}{}
		}
		assert.Equal(c, want, installed)
	}, 60*time.Second, 100*time.Millisecond,
		"probes should install within 60s")

	mu.Lock()
	require.NotNil(t, sink, "controlledSink was not installed")
	require.NotNil(t, prog, "loader.Program handle was not captured")
	mu.Unlock()

	client := &http.Client{Timeout: 30 * time.Second}
	doGet := func(path string) {
		t.Helper()
		resp, err := client.Get(baseURL + path)
		require.NoError(t, err, "GET %s", path)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", path)
	}

	// Issue the trigger: a chain of 128 nodes, each with a 4 KiB string.
	// Raw bytes ≈ 128 × (4096 + ~80) ≈ 533 KiB, well over the 512 KiB
	// cap (16 fragments × 32 KiB). The probe will emit fragments [0..15]
	// then refuse the 17th flush, take the continuation_aborted path,
	// and emit PARTIAL_ENTRY(last_seq=15).
	const triggerNodes = 128
	const triggerStrLen = 4 << 10
	doGet(fmt.Sprintf("/string_chain?iter=1&nodes=%d&str_len=%d",
		triggerNodes, triggerStrLen))

	// Wait for any drop notifications to land.
	var observedDrops []output.DropNotification
	collectTimer := time.NewTimer(2 * time.Second)
collect:
	for {
		select {
		case d := <-sink.drops:
			observedDrops = append(observedDrops, d)
		case <-collectTimer.C:
			break collect
		}
	}
	for i, d := range observedDrops {
		t.Logf("observed drop notification[%d]: reason=%d last_seq=%d probe_id=%d goid=%d entry_ktime_ns=%d",
			i, d.Drop_reason, d.Last_seq, d.Probe_id, d.Goid, d.Entry_ktime_ns)
	}

	// Filter to notifications matching the cap signature: PARTIAL_ENTRY
	// with last_seq = MAX_CONTINUATION_FRAGMENTS - 1.
	const expectLastSeq = uint16(15) // MAX_CONTINUATION_FRAGMENTS - 1
	var capDrops []output.DropNotification
	for _, d := range observedDrops {
		if output.DropReason(d.Drop_reason) == output.DropReasonPartialEntry &&
			d.Last_seq == expectLastSeq {
			capDrops = append(capDrops, d)
		}
	}
	require.Len(t, capDrops, 1,
		"expected exactly one PARTIAL_ENTRY(last_seq=%d) from the trigger; "+
			"observed %d total notifications, %d matching",
		expectLastSeq, len(observedDrops), len(capDrops))

	// Sanity: the side channel must not have lost notifications.
	assert.Equal(t, uint64(0), prog.DropNotifyLostAt(),
		"drop-notify side channel must not have lost notifications during the test")

	// Wait for fakeintake to receive the truncated snapshot.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		snaps := countSnapshotsForProbe(t, testServer.getLogs(), "capture_string_chain")
		assert.Greater(c, snaps, 0,
			"trigger snapshot for capture_string_chain should land "+
				"(16 fragments reassembled into a truncated event)")
	}, 10*time.Second, 100*time.Millisecond)

	// Recovery: a fresh small invocation must produce a snapshot.
	logsBeforeRecovery := len(testServer.getLogs())
	doGet("/bytes?iter=1&size=128")
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Greater(c, len(testServer.getLogs()), logsBeforeRecovery,
			"recovery: fresh /bytes invocation must produce a snapshot")
	}, 10*time.Second, 100*time.Millisecond)

	// Stop the subject process and drain.
	_ = proc.Process.Signal(os.Interrupt)
	_ = proc.Wait()
	sendUpdate(process.ProcessesUpdate{
		Removals: []process.ID{{PID: int32(pid)}},
	})
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Empty(c, m.DiagnosticsStates())
	}, 30*time.Second, 100*time.Millisecond,
		"diagnostics states should drain after process exit")

	t.Logf("collected %d snapshot logs total", len(testServer.getLogs()))
}

// countSnapshotsForProbe counts how many fakeintake logs are
// snapshots emitted by the given userspace probe ID. The probe ID is
// embedded in the snapshot JSON's debugger.snapshot.probe.id field.
func countSnapshotsForProbe(t *testing.T, logs []receivedLog, probeID string) int {
	t.Helper()
	var count int
	for _, log := range logs {
		// Each log body is a JSON array of snapshot envelopes; we
		// match on the embedded probe ID without fully unmarshaling
		// to avoid coupling to the schema.
		var envelopes []struct {
			Debugger struct {
				Snapshot struct {
					Probe struct {
						ID string `json:"id"`
					} `json:"probe"`
				} `json:"snapshot"`
			} `json:"debugger"`
		}
		if err := json.Unmarshal(log.body, &envelopes); err != nil {
			// Some logs may not be JSON arrays (e.g., singletons).
			// Try as a single envelope.
			var single struct {
				Debugger struct {
					Snapshot struct {
						Probe struct {
							ID string `json:"id"`
						} `json:"probe"`
					} `json:"snapshot"`
				} `json:"debugger"`
			}
			if err := json.Unmarshal(log.body, &single); err == nil {
				if single.Debugger.Snapshot.Probe.ID == probeID {
					count++
				}
			}
			continue
		}
		for _, e := range envelopes {
			if e.Debugger.Snapshot.Probe.ID == probeID {
				count++
			}
		}
	}
	return count
}
