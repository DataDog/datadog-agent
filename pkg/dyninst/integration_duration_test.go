// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestDurationThreshold exercises the runtime semantics of the
// @duration > X probe condition end-to-end. The condDuration_threshold
// probe on drop_tester's main.CaptureSleep fires only when the
// observed duration exceeds 100ms. We issue two sequential calls per
// iteration — one with a 1ms sleep, one with a 250ms sleep — and
// assert that the long call's snapshot arrives. The 250ms call must
// always produce a snapshot; if the 1ms call also produces one (which
// can happen on a heavily loaded host where 1ms+overhead exceeds the
// 100ms threshold), we treat the iteration as inconclusive and retry.
// We retry up to 10 times before giving up.
func TestDurationThreshold(t *testing.T) {
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

	const (
		shortSleepMs = 1
		longSleepMs  = 250
		maxAttempts  = 10
		// Time to wait for the long call's snapshot to arrive before
		// failing the test. The long call sleeps 250ms; the snapshot
		// uploader runs on its own cadence so allow generous slack.
		eventWait = 60 * time.Second
	)

	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-duration-test")
	defer cleanup()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)

	modCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
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

	client := &http.Client{Timeout: 30 * time.Second}
	doSleep := func(ms int) {
		url := fmt.Sprintf("%s/sleep?ms=%d", baseURL, ms)
		resp, err := client.Get(url)
		require.NoErrorf(t, err, "GET %s failed", url)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"unexpected status for %s", url)
	}

	longMarker := []byte(fmt.Sprintf(`"ms=%d"`, longSleepMs))
	shortMarker := []byte(fmt.Sprintf(`"ms=%d"`, shortSleepMs))

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Establish a baseline so we only consider snapshots that
		// arrived during this iteration.
		baseline := len(testServer.getLogs())

		// Sequential: 1ms first, then 250ms. Both calls must fire
		// before we start inspecting snapshots.
		doSleep(shortSleepMs)
		doSleep(longSleepMs)

		// Wait for the 250ms call's snapshot to land. Failure here
		// means the @duration > 100.0 condition didn't fire on a
		// call that should have triggered it — a real bug, not
		// timing noise. We let testify capture the assertion on
		// each tick so the final error message is informative.
		var sawLong bool
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			logs := testServer.getLogs()[baseline:]
			for _, log := range logs {
				if log.id != "condDuration_threshold" {
					continue
				}
				if bytes.Contains(log.body, longMarker) {
					sawLong = true
					return
				}
			}
			assert.True(c, false,
				"attempt %d: 250ms call did not produce a "+
					"condDuration_threshold snapshot within %s",
				attempt, eventWait)
		}, eventWait, 50*time.Millisecond,
			"250ms call must always produce a snapshot")
		require.True(t, sawLong)

		// Now scan the new snapshots and check the 1ms call did NOT
		// fire. If it did, the host was slow enough that the 1ms
		// call exceeded 100ms — log it and retry.
		var sawShort bool
		for _, log := range testServer.getLogs()[baseline:] {
			if log.id != "condDuration_threshold" {
				continue
			}
			if bytes.Contains(log.body, shortMarker) {
				sawShort = true
				break
			}
		}
		if !sawShort {
			t.Logf("attempt %d: clean run — only the 250ms call fired", attempt)
			return
		}
		t.Logf("attempt %d: 1ms call also exceeded 100ms threshold; retrying", attempt)
	}

	t.Fatalf(
		"after %d attempts the 1ms call always exceeded the 100ms "+
			"threshold; system too slow or threshold too tight",
		maxAttempts,
	)
}
