// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestCircuitBreaker(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf("cross-execution is not supported, running on %s, skipping %s", runtime.GOARCH, cfg.GOARCH)
				return
			}
			testCircuitBreaker(t, cfg)
		})
	}
}

func testCircuitBreaker(
	t *testing.T,
	progCfg testprogs.Config,
) {
	tempDir, cleanup := dyninsttest.PrepTmpDir(t, "dyninst-circuit-breaker-test")
	defer cleanup()

	testServer := newFakeAgent(t)
	t.Cleanup(testServer.s.Close)
	cfg, err := module.NewConfig(nil)
	require.NoError(t, err)
	cfg.DiskCacheConfig.DirPath = filepath.Join(tempDir, "disk-cache")
	cfg.LogUploaderURL = testServer.getLogsURL()
	cfg.DiagsUploaderURL = testServer.getDiagsURL()
	cfg.ActuatorConfig.CircuitBreakerConfig = actuator.CircuitBreakerConfig{
		Interval:          10 * time.Millisecond,
		PerProbeCPULimit:  0.1,
		AllProbesCPULimit: 0.5,
		InterruptOverhead: 2 * time.Microsecond,
	}
	cfg.ProbeTombstoneFilePath = filepath.Join(tempDir, "tombstone.json")
	var sendUpdate fakeProcessSubscriber
	cfg.TestingKnobs.ProcessSubscriberOverride = func(
		real module.ProcessSubscriber,
	) module.ProcessSubscriber {
		real.(*procsubscribe.Subscriber).Close() // prevent start from doing anything
		return &sendUpdate
	}
	m, err := module.NewModule(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(m.Close)

	bin := testprogs.MustGetBinary(t, "busyloop", progCfg)
	probes := testprogs.MustGetProbeDefinitions(t, "busyloop")

	// Launch the sample service.
	t.Logf("launching busyloop")
	ctx := context.Background()
	sampleProc, sampleStdin := dyninsttest.StartProcess(
		ctx, t, tempDir, bin,
		"1" /*round_cnt*/, "20" /*round_sec*/, "3", /*concurrency*/
	)
	defer func() {
		_ = sampleProc.Process.Kill()
		_ = sampleProc.Wait()
	}()

	exe, err := process.ResolveExecutable(kernel.ProcFSRoot(), int32(sampleProc.Process.Pid))
	require.NoError(t, err)
	const runtimeID = "foo"
	sendUpdate(process.ProcessesUpdate{
		Updates: []process.Config{
			{
				Info: process.Info{
					ProcessID:  process.ID{PID: int32(sampleProc.Process.Pid)},
					Executable: exe,
					Service:    "busyloop",
				},
				RuntimeID:         runtimeID,
				Probes:            probes,
				ShouldUploadSymDB: false,
			},
		},
	})

	// Start the busy loop.
	sampleStdin.Write([]byte("\n"))

	deadline := time.Now().Add(10 * time.Second)
	var diags []receivedDiag
	for time.Now().Before(deadline) {
		diags = testServer.getDiags()
		if len(diags) > 0 && diags[len(diags)-1].diagnosticMessage.Debugger.Diagnostic.Status == uploader.StatusError {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Greater(t, len(diags), 0)
	d := diags[len(diags)-1].diagnosticMessage.Debugger.Diagnostic
	require.Equal(t, d.Status, uploader.StatusError)
	e := d.DiagnosticException
	require.NotNil(t, e)
	require.Equal(t, "ExecutionFailed", e.Type)
	require.Contains(t, e.Message, "exceeded CPU limit")
}
