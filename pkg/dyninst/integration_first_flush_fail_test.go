// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// TestFirstFlushFailDropNotification deterministically exercises the case
// where a probe invocation overflows the per-CPU scratch buffer and its
// very first scratch_buf_flush_and_continue call fails because the
// output ringbuf is saturated. Exercises both:
//
//   - Variant A (PARTIAL_RETURN): entry fits in one fragment, return
//     overflows and its first flush fails.
//   - Variant B (PARTIAL_ENTRY): entry overflows directly, first flush
//     fails.
//
// The test installs a controlledSink that blocks HandleEvent on demand.
// While the sink is blocked, the dispatcher's output-reader goroutine
// stalls and the output ringbuf fills up. The test polls
// OutputReader().AvailableBytes() until the ringbuf is saturated, then
// issues exactly one trigger request whose probe invocation will hit
// the saturated ringbuf on its first mid-chase flush. The test reads
// the resulting drop notification from the side channel (which has its
// own consumer goroutine and is not blocked) and asserts on userspace
// invariants once the sink is released and the system has drained.
//
// The bug being verified: the continuation_aborted branch in event.c
// emits PARTIAL_* with last_submitted_seq still LAST_SUBMITTED_SEQ_NONE
// (0xFFFF) when the very first scratch_buf_flush_and_continue fails.
// Userspace's NotePartial computes entryExpected = lastSeq + 1 with
// uint16, which wraps to 0; per buffer.go semantics that's "still
// assembling, total unknown", so the entry sits in the eventbuf
// forever — defeating the notification's purpose.
//
// The test's diagnostic invariant: NO drop notification carries the
// sentinel last_seq=0xFFFF. With the fix this holds; without the fix
// the test fails on observed PARTIAL_*(last_seq=0xFFFF) notifications.
//
// Skipped pending the BPF fix in the next commit. Removing the skip
// locally and running against the unfixed BPF reproduces the bug
// deterministically: PARTIAL_*(last_seq=65535) notifications surface
// the failed assertion. The follow-up fix-commit removes this skip.
func TestFirstFlushFailDropNotification(t *testing.T) {
	t.Skip("DEBUG-XXXX: pending BPF fix in the next commit; see PR review")

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

	t.Run("variant_A_return_side_first_flush_fails", func(t *testing.T) {
		runFirstFlushFailScenario(t, cfg, firstFlushFailVariant{
			// Entry fits easily; return needs continuation. While the
			// output ringbuf is saturated the trigger's return-side
			// flush hits the first-flush-fails path. Post-fix BPF
			// either suppresses the notification or emits RETURN_LOST,
			// never PARTIAL_RETURN(last_seq=0xFFFF). Pre-fix BPF emits
			// PARTIAL_RETURN(last_seq=0xFFFF), which userspace silently
			// strands in the eventbuf.
			triggerPath: "/bytes_return?iter=1&size=100&return_size=40000",
		})
	})

	t.Run("variant_B_entry_side_first_flush_fails", func(t *testing.T) {
		runFirstFlushFailScenario(t, cfg, firstFlushFailVariant{
			// Entry needs continuation directly. While the output
			// ringbuf is saturated the trigger's entry-side flush hits
			// the first-flush-fails path. Post-fix BPF suppresses the
			// notification entirely (entry probe with no fragments has
			// nothing in userspace to truncate). Pre-fix BPF emits
			// PARTIAL_ENTRY(last_seq=0xFFFF), which userspace silently
			// strands.
			triggerPath: "/bytes?iter=1&size=40000",
		})
	})
}

type firstFlushFailVariant struct {
	triggerPath string
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

	// blocking is non-zero while HandleEvent should park.
	blocking atomic.Bool
	// release is closed by the test to wake any parked HandleEvent
	// callers and let new calls fall through.
	release chan struct{}
	// drops receives every drop notification observed. Buffered so
	// the dispatcher's runDropNotify goroutine doesn't block.
	drops chan output.DropNotification
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
		// in the BPF side. We ignore here and let the test's
		// assertions surface the discrepancy.
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

// startBlocking begins parking HandleEvent callers.
func (c *controlledSink) startBlocking() { c.blocking.Store(true) }

// stopBlocking releases any parked callers and lets new calls fall
// through. Idempotent.
func (c *controlledSink) stopBlocking() {
	c.blocking.Store(false)
	select {
	case <-c.release:
	default:
		close(c.release)
	}
}

func runFirstFlushFailScenario(t *testing.T, cfg testprogs.Config, variant firstFlushFailVariant) {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-first-flush-fail")
	defer cleanup()

	// Match the existing TestDropNotifications ringbuf size so the
	// filler-request count is predictable. 128 KiB is small enough to
	// saturate quickly and large enough to host the trigger event's
	// first fragment.
	const ringbufBytes = 128 << 10

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	modCfg.TestingKnobs.LoaderOptions = []loader.Option{
		loader.WithRingBufSize(ringbufBytes),
	}
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

	// Wire up the sink override, loader-ready, and program-loaded callbacks.
	var (
		mu     sync.Mutex
		sink   *controlledSink
		prog   *loader.Program
		ldr    *loader.Loader
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

	// Launch drop_tester.
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

	// At this point the sink override has run (one program loaded).
	mu.Lock()
	require.NotNil(t, sink, "controlledSink was not installed")
	require.NotNil(t, prog, "loader.Program handle was not captured")
	require.NotNil(t, ldr, "loader.Loader handle was not captured")
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

	// Step 1: Begin blocking the dispatcher's output reader. From this
	// point forward, any probe-event records the BPF program writes
	// will pile up in the output ringbuf without being drained.
	sink.startBlocking()

	// Step 2: Saturate the output ringbuf with filler invocations.
	// Each /bytes?size=512 call produces a single ~600-byte ringbuf
	// record. We send enough requests to reach >90% of the ringbuf
	// capacity (the kernel rejects new reservations once free space
	// drops below the requested record size). We poll AvailableBytes
	// after every batch to guard against either over- or under-shooting.
	const fillerSize = 512
	const fillerBatch = 32
	maxBatches := (ringbufBytes / (fillerSize * fillerBatch)) + 4
	saturated := false
	for batch := 0; batch < maxBatches; batch++ {
		doGet(fmt.Sprintf("/bytes?iter=%d&size=%d", fillerBatch, fillerSize))
		// Brief pause so the BPF probes catch up.
		time.Sleep(50 * time.Millisecond)
		if avail := ldr.OutputReader().AvailableBytes(); avail >= ringbufBytes-(2*fillerSize) {
			t.Logf("ringbuf saturated after %d batches (avail=%d/%d)", batch+1, avail, ringbufBytes)
			saturated = true
			break
		}
	}
	require.True(t, saturated, "failed to saturate output ringbuf within budget")

	// Step 3: Issue the trigger request. The probe invocation will need
	// continuation; its first flush will fail because the ringbuf is
	// full.
	doGet(variant.triggerPath)

	// Step 4: Wait briefly for any drop notifications. We collect for
	// a fixed window rather than gating on the first arrival because
	// some variants (entry-side first-flush-fails, post-fix) expect
	// zero notifications, and we need to give BPF time to *not*
	// emit one.
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

	// Step 5: Release the sink and let the dispatcher drain. From here
	// on, the test asserts on the userspace state.
	sink.stopBlocking()

	// Give the dispatcher time to drain the ringbuf and the eventbuf
	// time to apply any drop notification we observed.
	time.Sleep(2 * time.Second)

	for i, d := range observedDrops {
		t.Logf("observed drop notification[%d]: reason=%d last_seq=%d probe_id=%d goid=%d entry_ktime_ns=%d",
			i, d.Drop_reason, d.Last_seq, d.Probe_id, d.Goid, d.Entry_ktime_ns)
	}
	// Critical post-fix invariant: BPF must never send a drop
	// notification with last_seq=LAST_SUBMITTED_SEQ_NONE (0xFFFF).
	// PARTIAL_* with that sentinel would cause userspace's NotePartial
	// to wrap entryExpected to 0 and strand the entry indefinitely.
	// The fix in the continuation_aborted branch gates that path; this
	// assertion fails on the unfixed BPF.
	for i, d := range observedDrops {
		assert.NotEqual(t, uint16(0xFFFF), d.Last_seq,
			"drop notification[%d] must not carry sentinel last_seq=0xFFFF (reason=%d, probe_id=%d)",
			i, d.Drop_reason, d.Probe_id)
	}

	// Sanity: the drop-notify side channel itself must not have dropped
	// notifications mid-test, otherwise the sentinel-last_seq assertion
	// above is unreliable.
	assert.Equal(t, uint64(0), prog.DropNotifyLostAt(),
		"drop-notify side channel must not have lost notifications during the test")
	// The eventbuf state is intentionally not asserted here — the
	// filler workload generates LOG_PROBE entry+return pairs whose
	// returns naturally orphan in userspace under saturation, so the
	// buffer is generally non-empty. The bug-detecting assertion is
	// the "no sentinel last_seq" check above; that assertion is
	// precise and orthogonal to the filler-induced noise.
	t.Logf("end-of-test eventbuf state: Len=%d Bytes=%d budget.Used=%d",
		sink.buffer.Len(), sink.buffer.Bytes(), sink.budget.Used())

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

	t.Logf("collected %d snapshot logs", len(testServer.getLogs()))
}
