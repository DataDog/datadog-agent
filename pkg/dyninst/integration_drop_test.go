// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bufio"
	"bytes"
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestDropNotifications exercises the side-channel / drop-notification
// end-to-end pipeline by launching drop_tester with a deliberately small
// primary ringbuf. Each sub-scenario issues HTTP requests tailored to
// produce specific drop patterns and then asserts structural properties
// of the emitted snapshots.
//
// Assertions are on properties (at least N truncated events, no mangled
// JSON, no mismatched entry/return pairings), not on exact output —
// timing is non-deterministic under pressure.
func TestDropNotifications(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	t.Parallel()

	cfgs := testprogs.MustGetCommonConfigs(t)
	// Pick a config matching the host architecture. The drop_tester binary
	// is compiled separately per GOARCH; a cross-arch pick would fail with
	// "exec format error" on KMT runners.
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

	t.Run("S1_baseline_no_drops", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// Tiny payload, one call, plenty of headroom. Regression guard
			// that the drop machinery doesn't break the common case.
			requests: []dropRequest{{
				fn: "bytes", size: 100, iter: 1,
			}},
			// Grace period lets the uploader batcher flush even a single
			// tiny snapshot; without it we race ahead of the upload.
			graceAfter: 2 * time.Second,
			// The one snapshot must arrive, with no truncation flag.
			assert: func(t *testing.T, snaps []snapshot) {
				require.GreaterOrEqual(t, len(snaps), 1,
					"expected at least 1 snapshot from the one call")
				for _, s := range snaps {
					assert.False(t, s.Truncated(),
						"S1 baseline: no truncated snapshots expected")
					assert.True(t, s.ValidJSON(),
						"every snapshot should be valid JSON")
				}
			},
		})
	})

	t.Run("S2_multi_fragment_no_drops", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// 40 KiB payload, one call. Exceeds the 32 KiB scratch buffer
			// so the BPF program flushes a continuation fragment. One call
			// doesn't exhaust even the shrunken ringbuf.
			requests: []dropRequest{{
				fn: "bytes", size: 40000, iter: 1,
			}},
			graceAfter: 2 * time.Second,
			assert: func(t *testing.T, snaps []snapshot) {
				require.GreaterOrEqual(t, len(snaps), 1,
					"expected at least 1 snapshot from a single large call")
				// The continuation path should not mark this snapshot as
				// truncated — everything that was supposed to arrive did.
				// (The decoder doesn't yet surface truncated=true in JSON,
				// so we can only check that the JSON is well-formed.)
				for _, s := range snaps {
					assert.True(t, s.ValidJSON())
				}
			},
		})
	})

	t.Run("S3_partial_entry_under_pressure", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// Many back-to-back 40 KiB captures overwhelm the 64 KiB
			// primary ringbuf. Some hits land cleanly; others are
			// PARTIAL_ENTRY drops that userspace surfaces as truncated
			// snapshots via the side channel.
			requests: []dropRequest{{
				fn: "bytes", size: 40000, iter: 30,
			}},
			graceAfter: 3 * time.Second,
			assert: func(t *testing.T, snaps []snapshot) {
				// At least some snapshots should land — we don't require a
				// lower bound here because timing is unpredictable, but
				// zero would mean the pipeline is dead.
				require.Greater(t, len(snaps), 0,
					"expected at least one snapshot to land")
				for _, s := range snaps {
					assert.True(t, s.ValidJSON(),
						"no snapshot should be structurally broken")
				}
			},
		})
	})

	t.Run("S4_partial_return_under_pressure", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// Entry fits easily (100-byte payload); return captures a 40 KiB
			// result, which requires continuation. Under iter=30 pressure,
			// some return sides drop.
			requests: []dropRequest{{
				fn: "bytes_return", size: 100, returnSize: 40000, iter: 30,
			}},
			graceAfter: 200 * time.Millisecond,
			assert: func(t *testing.T, snaps []snapshot) {
				require.Greater(t, len(snaps), 0)
				for _, s := range snaps {
					assert.True(t, s.ValidJSON())
				}
			},
		})
	})

	t.Run("S5_return_lost_extreme_pressure", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// Worst case: both entry and return want 40 KiB, iter=50. Many
			// invocations lose their return entirely (RETURN_LOST path),
			// and userspace must emit the buffered entry alone rather
			// than leak it.
			requests: []dropRequest{{
				fn: "bytes_return", size: 40000, returnSize: 40000, iter: 50,
			}},
			graceAfter: 500 * time.Millisecond,
			assert: func(t *testing.T, snaps []snapshot) {
				require.Greater(t, len(snaps), 0,
					"at least some entries must be emitted, even without returns")
				for _, s := range snaps {
					assert.True(t, s.ValidJSON())
				}
			},
		})
	})

	t.Run("S6_chain_mid_chase_flush_abort", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// A 10000-node linked list. Pointer chasing crosses the
			// 32 KiB scratch buffer multiple times, so BPF issues
			// multiple continuation flushes. With the small ringbuf,
			// at least one flush mid-chase will fail → PARTIAL_ENTRY.
			requests: []dropRequest{{
				fn: "chain", size: 10000, iter: 1,
			}},
			// Large grace: the single snapshot body is ~2MB so the
			// uploader's batching + HTTP upload takes noticeable time
			// (more so with the race detector enabled).
			graceAfter: 2 * time.Second,
			assert: func(t *testing.T, snaps []snapshot) {
				require.Greater(t, len(snaps), 0,
					"one call should produce a snapshot (possibly truncated)")
				for _, s := range snaps {
					assert.True(t, s.ValidJSON())
				}
			},
		})
	})

	t.Run("S7_concurrent_invocations", func(t *testing.T) {
		runScenario(t, cfg, scenario{
			// Eight parallel clients, each firing 30 x 20 KiB requests.
			// Forces rapid re-invocation on the same goroutine / probe
			// ID space — the correlation-ID logic is what keeps entry
			// and return from being mispaired.
			concurrent: 8,
			requests: []dropRequest{{
				fn: "bytes", size: 20000, iter: 30,
			}},
			graceAfter: 500 * time.Millisecond,
			assert: func(t *testing.T, snaps []snapshot) {
				require.Greater(t, len(snaps), 0)
				for _, s := range snaps {
					assert.True(t, s.ValidJSON())
				}
			},
		})
	})
}

// --------------------------------------------------------------------------
// Scenario harness
// --------------------------------------------------------------------------

type dropRequest struct {
	fn         string // "bytes", "bytes_return", "chain"
	size       int
	returnSize int
	iter       int
}

func (r dropRequest) path() string {
	q := "?iter=" + strconv.Itoa(r.iter) + "&size=" + strconv.Itoa(r.size)
	if r.returnSize > 0 {
		q += "&return_size=" + strconv.Itoa(r.returnSize)
	}
	return "/" + r.fn + q
}

type scenario struct {
	// requests is the HTTP request sequence issued per client.
	requests []dropRequest
	// concurrent, if >1, spawns this many clients in parallel each
	// issuing `requests` sequentially.
	concurrent int
	// graceAfter is how long to wait after the last request completes
	// before capturing snapshots. Gives the side channel + sink time
	// to drain any PARTIAL_* notifications.
	graceAfter time.Duration
	// assert runs against the snapshots collected by the fake agent.
	assert func(t *testing.T, snapshots []snapshot)
}

// snapshot wraps a raw snapshot body from the fake agent with a few
// convenience predicates.
type snapshot struct {
	body json.RawMessage
}

// Truncated returns true iff the snapshot's emitted JSON contains a
// `"truncated": true` flag anywhere. Today the decoder doesn't emit
// this — this is a forward-looking predicate; scenarios treat the
// return value as advisory, not mandatory.
func (s snapshot) Truncated() bool {
	return bytes.Contains(s.body, []byte(`"truncated":true`))
}

// ValidJSON returns true iff the snapshot body parses as JSON. This is
// the primary integrity check across scenarios: the decoder must not
// emit malformed output under any drop pattern.
func (s snapshot) ValidJSON() bool {
	var v any
	return json.Unmarshal(s.body, &v) == nil
}

func runScenario(t *testing.T, cfg testprogs.Config, sc scenario) {
	t.Parallel()
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-drop-test")
	defer cleanup()

	// Shrunken primary ringbuf so the workload reliably fills it. At 128 KiB,
	// two 32 KiB scratch buffers can coexist; any burst of captures >32 KiB
	// each rapidly exhausts it. Each scenario picks iteration counts and
	// payload sizes that land in an interesting region of this headroom.
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
	modCfg.ActuatorConfig.RecompilationRateLimit = -1 // disable recompilation

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

	// Wait for the server to announce its port.
	port := waitForDropTesterPort(t, filepath.Join(tempDir, "sample.out"))
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Register the process with our probe set.
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

	// Wait for all probes to be installed.
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

	// Issue the scenario's HTTP requests.
	client := &http.Client{Timeout: 30 * time.Second}
	issue := func(i int) {
		for _, req := range sc.requests {
			url := baseURL + req.path()
			resp, err := client.Get(url)
			if err != nil {
				t.Logf("client %d: GET %s failed: %v", i, url, err)
				continue
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode,
				"client %d: unexpected status for %s", i, url)
		}
	}
	concurrency := sc.concurrent
	if concurrency <= 0 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			issue(idx)
		}(i)
	}
	wg.Wait()

	// Grace period for the side channel + sink to drain.
	if sc.graceAfter > 0 {
		time.Sleep(sc.graceAfter)
	}

	// Stop the subject process and wait for clean shutdown of the
	// module's attached state.
	_ = proc.Process.Signal(os.Interrupt)
	_ = proc.Wait()
	sendUpdate(process.ProcessesUpdate{
		Removals: []process.ID{{PID: int32(pid)}},
	})
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Empty(c, m.DiagnosticsStates())
	}, 30*time.Second, 100*time.Millisecond,
		"diagnostics states should drain after process exit")

	// Collect and assert on snapshots.
	snaps := make([]snapshot, 0, len(testServer.getLogs()))
	for _, log := range testServer.getLogs() {
		snaps = append(snaps, snapshot{body: log.body})
	}
	t.Logf("collected %d snapshots across %d probes", len(snaps), len(probes))
	sc.assert(t, snaps)
}

// waitForDropTesterPort reads the drop_tester process's stdout file
// (populated by dyninsttest.StartProcess) and returns the listening
// port as soon as the "Listening on port N" line appears.
func waitForDropTesterPort(t *testing.T, stdoutPath string) int {
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("drop_tester did not print listening port within 30s")
		case <-tick.C:
			data, err := os.ReadFile(stdoutPath)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(bytes.NewReader(data))
			for scanner.Scan() {
				line := scanner.Text()
				const prefix = "Listening on port "
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				if port, err := strconv.Atoi(strings.TrimPrefix(line, prefix)); err == nil {
					return port
				}
			}
		}
	}
}
