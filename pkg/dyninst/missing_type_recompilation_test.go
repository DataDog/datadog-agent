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
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module/tombstone"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestMissingTypeRecompilation verifies the full missing-types feedback loop:
// decode → discover missing types → recompile with additional types → capture actual values.
//
// The testAny probe on the sample binary probes main.testAny(a any). Types like
// otherFirstBehavior, otherSecondBehavior, and lib_v2.V2Type produce
// "notCapturedReason": "missing type information" on the first run because they
// are not in the IR. After the actuator triggers recompilation with
// WithAdditionalTypes, those types should be fully captured in a subsequent run.
//
// The test loops, launching a new process each iteration, until no events
// contain missing type information — proving the feedback loop converges.
func TestMissingTypeRecompilation(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	current := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, current) })

	cfgs := testprogs.MustGetCommonConfigs(t)

	// Use the first config that matches our GOARCH.
	var cfg testprogs.Config
	cfgIdx := slices.IndexFunc(cfgs, func(c testprogs.Config) bool {
		return c.GOARCH == runtime.GOARCH && strings.Contains(c.GOTOOLCHAIN, "go1.26")
	})
	require.NotEqual(t, -1, cfgIdx, "no config for current arch and go1.26")
	cfg = cfgs[cfgIdx]
	bin := testprogs.MustGetBinary(t, "sample", cfg)

	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-missing-type-recomp")
	defer cleanup()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)

	// Technically in the worst case we could detach the program on each
	// missing type and there are up to this many.
	const maxRecompilations = 6

	moduleCfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	moduleCfg.ActuatorConfig.RecompilationRateBurst = maxRecompilations
	moduleCfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	moduleCfg.LogUploaderURL = testServer.getLogsURL()
	moduleCfg.DiagsUploaderURL = testServer.getDiagsURL()
	moduleCfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	moduleCfg.TestingKnobs.TombstoneSleepKnobs = tombstone.WaitTestingKnobs{
		BackoffPolicy: &backoff.ExpBackoffPolicy{
			MaxBackoffTime: time.Millisecond.Seconds(),
		},
	}

	var sendUpdate fakeProcessSubscriber
	moduleCfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close()
		return &sendUpdate
	}

	m, err := module.NewModule(moduleCfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	// Get only the testAny probe.
	allProbes := testprogs.MustGetProbeDefinitions(t, "sample")
	var testAnyProbe ir.ProbeDefinition
	for _, p := range allProbes {
		if p.GetID() == "testAny" {
			testAnyProbe = p
			break
		}
	}
	require.NotNil(t, testAnyProbe, "testAny probe not found")
	probes := []ir.ProbeDefinition{testAnyProbe}

	const service = "sample"

	// runProcess launches the sample binary, sends a process update to the
	// module, waits for probes to be installed, triggers execution, waits for
	// the process to exit, then removes the process and waits for the actuator
	// to settle (numPrograms == 0). Returns the logs produced during this run.
	runProcess := func(runName string) []receivedLog {
		t.Helper()
		ctx := context.Background()
		proc, stdin := dyninsttest.StartProcess(ctx, t, tempDir, bin)
		pid := proc.Process.Pid
		t.Logf("[%s] launched sample with pid %d", runName, pid)
		defer func() {
			_ = proc.Process.Kill()
			_ = proc.Wait()
		}()

		exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(pid))
		require.NoError(t, err)

		// Track how many logs existed before this run.
		logsBefore := len(testServer.getLogs())

		runtimeID := "runtime-" + runName
		sendUpdate(process.ProcessesUpdate{
			Updates: []process.Config{{
				Info: process.Info{
					ProcessID:  process.ID{PID: int32(pid)},
					Executable: exe,
					Service:    service,
				},
				RuntimeID: runtimeID,
				Probes:    slices.Clone(probes),
			}},
		})

		// Wait for probes to be installed.
		allProbeIDs := make(map[string]struct{}, len(probes))
		for _, p := range probes {
			allProbeIDs[p.GetID()] = struct{}{}
		}
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			installed := make(map[string]struct{})
			for _, d := range testServer.getDiags() {
				if d.diagnosticMessage.Debugger.RuntimeID != runtimeID {
					continue
				}
				if d.diagnosticMessage.Debugger.Status == uploader.StatusInstalled {
					installed[d.diagnosticMessage.Debugger.ProbeID] = struct{}{}
				}
			}
			assert.Equal(c, allProbeIDs, installed)
		}, 180*time.Second, 100*time.Millisecond)

		// Trigger function calls, give the uploader time to flush events,
		// then wait for the process to exit.
		t.Logf("[%s] triggering function calls", runName)
		stdin.Write([]byte("\n"))
		require.NoError(t, proc.Wait())

		// Remove the process and wait for the actuator to fully settle. After
		// removal and any in-flight recompilation attempts, numPrograms should
		// reach 0.
		sendUpdate(process.ProcessesUpdate{
			Removals: []process.ID{{PID: int32(pid)}},
		})
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			stats := m.GetStats()
			actuatorStats, ok := stats["actuator"].(map[string]any)
			if !assert.True(c, ok, "actuator stats missing") {
				return
			}
			assert.Equal(c, uint64(0), actuatorStats["numPrograms"],
				"expected numPrograms == 0 after cleanup")
			assert.Equal(c, uint64(0), actuatorStats["numProcesses"],
				"expected numProcesses == 0 after cleanup")
		}, 30*time.Second, 100*time.Millisecond)

		// Return only the logs from this run.
		allLogs := testServer.getLogs()
		runLogs := allLogs[logsBefore:]
		t.Logf("[%s] got %d events", runName, len(runLogs))
		return runLogs
	}

	// Loop: run the sample binary until we see no missing type information.
	// The first run should have missing types. The actuator records them as
	// discoveredTypes for the service, so subsequent runs compile with those
	// types included. We expect convergence within a small number of runs.
	var firstRunLogs []receivedLog
	for i := 1; i <= maxRecompilations; i++ {
		runName := fmt.Sprintf("run%d", i)
		logs := runProcess(runName)
		require.NotEmpty(t, logs, "[%s] expected at least one event", runName)

		missingCount := 0
		for _, log := range logs {
			if bytes.Contains(log.body, []byte(`"missing type information"`)) {
				missingCount++
			}
		}
		t.Logf("[%s] %d events total, %d with missing types", runName, len(logs), missingCount)

		if i == 1 {
			firstRunLogs = logs
			require.Greater(t, missingCount, 0,
				"first run should have missing types to prove the test is meaningful")
			continue
		}

		if missingCount == 0 {
			t.Logf("converged after %d runs", i)
			// Verify we got at least as many events as the first run.
			require.GreaterOrEqual(t, len(logs), len(firstRunLogs),
				"final run should produce at least as many events as the first")

			// Verify the actuator recorded at least one type recompilation.
			stats := m.GetStats()
			actuatorStats := stats["actuator"].(map[string]any)
			require.Greater(t, actuatorStats["typeRecompilationsTriggered"], uint64(0),
				"expected at least one type recompilation to have been triggered")
			return
		}
	}
	t.Fatalf("still had missing types after %d runs", maxRecompilations)
}
