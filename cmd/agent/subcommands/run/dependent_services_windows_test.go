// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestServicedefIsEnabled(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_config.enabled", true, model.SourceDefault)

	svc := Servicedef{
		name: "process",
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
	}

	t.Run("enabled by config when not gated", func(t *testing.T) {
		assert.True(t, svc.IsEnabled(false, false))
		assert.True(t, svc.IsEnabled(false, true))
	})

	t.Run("not suppressed when gated but no definition file", func(t *testing.T) {
		svc.procmgrDefinitionFile = processProcmgrDefinitionFile
		withProcmgrInstallRoot(t, t.TempDir(), func() {
			assert.True(t, svc.IsEnabled(true, true))
		})
	})
}

func TestServicedefIsEnabled_procmgrSuppression(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_config.enabled", true, model.SourceDefault)
	cfg.Set("process_manager.enabled", true, model.SourceDefault)

	svc := Servicedef{
		name:                  "process",
		procmgrDefinitionFile: processProcmgrDefinitionFile,
		configKeys: map[string]model.Reader{
			"process_config.enabled": cfg,
		},
	}

	t.Run("not suppressed when processes.d file missing", func(t *testing.T) {
		installRoot := t.TempDir()
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.True(t, svc.IsEnabled(true, true))
		})
	})

	t.Run("suppressed when processes.d file exists and procmgr started", func(t *testing.T) {
		installRoot := writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile)
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.False(t, svc.IsEnabled(true, true))
		})
	})

	t.Run("falls back to legacy SCM when procmgr unavailable", func(t *testing.T) {
		installRoot := writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile)
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.True(t, svc.IsEnabled(true, false))
		})
	})

	// An installer run can write the definition after the startup pass classified this
	// service. The classification is config-only, so the service is still gated and the
	// decision uses the real dd-procmgr-service outcome instead of an assumed "not
	// started", which would have started a legacy service procmgr is about to supervise.
	t.Run("definition appearing late is decided against the real procmgr outcome", func(t *testing.T) {
		cfg.Set("process_manager.enabled", true, model.SourceDefault)
		require.True(t, svc.needsProcmgrStartupGate(cfg),
			"gating must not depend on the definition file existing yet")

		installRoot := writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile)
		withProcmgrInstallRoot(t, installRoot, func() {
			assert.False(t, svc.IsEnabled(true, true))
		})
	})
}

func TestServicedefIsEnabled_procmgrManagedServices(t *testing.T) {
	cfg := configmock.New(t)
	cfg.Set("process_manager.enabled", true, model.SourceDefault)

	cases := []struct {
		name    string
		defFile string
		config  map[string]bool
	}{
		{
			name:    "par",
			defFile: parProcmgrDefinitionFile,
			config:  map[string]bool{"private_action_runner.enabled": true},
		},
		{
			name:    "otel",
			defFile: ddotProcmgrDefinitionFile,
			config:  map[string]bool{"otelcollector.enabled": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installRoot := writeProcmgrDefinitionFile(t, tc.defFile)
			withProcmgrInstallRoot(t, installRoot, func() {
				keys := make(map[string]model.Reader, len(tc.config))
				for key := range tc.config {
					keys[key] = cfg
					cfg.Set(key, tc.config[key], model.SourceDefault)
				}
				svc := Servicedef{
					name:                  tc.name,
					procmgrDefinitionFile: tc.defFile,
					configKeys:            keys,
				}
				assert.True(t, svc.IsEnabled(true, false))
			})
		})
	}
}

// The suppression mechanism is wired here, but it must not apply to process-agent yet: the
// fleet template still ships auto_start: false, so suppressing the SCM service would leave
// no process-agent running at all on a default install. Both halves flip together, later.
func TestProcessServiceNotYetProcmgrManaged(t *testing.T) {
	coreConf := configmock.New(t)
	sysprobeConf := configmock.NewSystemProbe(t)

	svc, ok := findService(subservices(coreConf, sysprobeConf), "process")
	require.True(t, ok)
	require.Empty(t, svc.procmgrDefinitionFile,
		"wiring process-agent suppression requires flipping auto_start in the fleet template in the same change")
}

// stopDependentServices must not run its stop loop while a startup pass is still in
// flight, or a service that pass is mid-Start on would survive agent shutdown.
//
// The assertion is the order the two events are recorded in, not how long either took,
// so the test neither depends on the scheduler nor passes by default when the joining
// goroutine is slow to run.
func TestStopDependentServicesJoinsStartupPass(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, event)
	}

	release := make(chan struct{})
	dependentServicesStartup.Add(1)
	go func() {
		defer dependentServicesStartup.Done()
		<-release
		record("startup pass finished")
	}()

	waiting := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		close(waiting)
		dependentServicesStartup.Wait()
		record("shutdown joined")
		close(joined)
	}()

	// Hold the startup pass until the joining goroutine is running. A Wait that failed
	// to block would then record first and fail the ordering check, rather than the
	// test passing because that goroutine never got scheduled in time.
	<-waiting
	close(release)
	<-joined

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"startup pass finished", "shutdown joined"}, order,
		"shutdown must not proceed until the startup pass has finished")
}

func TestServicedefShouldStop(t *testing.T) {
	assert.True(t, (&Servicedef{shouldShutdown: true}).ShouldStop())
	assert.False(t, (&Servicedef{shouldShutdown: false}).ShouldStop())
}

func TestFindService(t *testing.T) {
	svcs := []Servicedef{{name: "apm"}, {name: "procmgr"}}
	svc, ok := findService(svcs, "procmgr")
	assert.True(t, ok)
	assert.Equal(t, "procmgr", svc.name)

	_, ok = findService(svcs, "missing")
	assert.False(t, ok)
}

func TestStartProcmgrIfEnabled(t *testing.T) {
	assert.False(t, startProcmgrIfEnabled(context.Background(), Servicedef{name: "procmgr"}))
}

func TestWaitForProcmgrStartupOutcome(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		state    svc.State
		err      error
		suppress bool
	}{
		{name: "suppresses when procmgr is running", ctx: context.Background(), state: svc.Running, suppress: true},
		{name: "falls back when procmgr is stopped", ctx: context.Background(), state: svc.Stopped, suppress: false},
		{name: "falls back when procmgr stops pending", ctx: context.Background(), state: svc.StopPending, suppress: false},
		{name: "suppresses when still start pending at timeout", ctx: context.Background(), state: svc.StartPending, err: context.DeadlineExceeded, suppress: true},
		{name: "suppresses when the state query fails", ctx: context.Background(), err: errors.New("access denied"), suppress: true},
		// Shutdown stops dd-procmgr-service, so a wait that outlived the agent would see
		// Stopped and tell the caller to start the legacy service the stop pass just stopped.
		{name: "suppresses when the agent is shutting down", ctx: cancelledContext(), state: svc.Stopped, suppress: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubStartPendingExit(t, func(ctx context.Context, serviceName string, currentState svc.State) (svc.State, error) {
				assert.Equal(t, "dd-procmgr-service", serviceName)
				assert.Equal(t, svc.StartPending, currentState)
				_, hasDeadline := ctx.Deadline()
				assert.True(t, hasDeadline, "the wait must be bounded")
				return tt.state, tt.err
			})
			assert.Equal(t, tt.suppress, waitForProcmgrStartupOutcome(tt.ctx, "dd-procmgr-service"))
		})
	}
}

func TestServicedefNeedsProcmgrStartupGate(t *testing.T) {
	cfg := configmock.New(t)
	svc := Servicedef{
		procmgrDefinitionFile: processProcmgrDefinitionFile,
	}

	t.Run("false without definition file field", func(t *testing.T) {
		cfg.Set("process_manager.enabled", true, model.SourceDefault)
		apmSvc := Servicedef{name: "apm"}
		assert.False(t, apmSvc.needsProcmgrStartupGate(cfg))
	})

	t.Run("false when process manager disabled", func(t *testing.T) {
		cfg.Set("process_manager.enabled", false, model.SourceDefault)
		withProcmgrInstallRoot(t, writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile), func() {
			assert.False(t, svc.needsProcmgrStartupGate(cfg))
		})
	})

	// Gating is config-only on purpose, so that processes.d is read exactly once per
	// startup pass, at the decision point, rather than here and again in IsEnabled.
	t.Run("true regardless of whether the processes.d file exists yet", func(t *testing.T) {
		cfg.Set("process_manager.enabled", true, model.SourceDefault)
		withProcmgrInstallRoot(t, t.TempDir(), func() {
			assert.True(t, svc.needsProcmgrStartupGate(cfg))
		})
		withProcmgrInstallRoot(t, writeProcmgrDefinitionFile(t, processProcmgrDefinitionFile), func() {
			assert.True(t, svc.needsProcmgrStartupGate(cfg))
		})
	})
}

func stubStartPendingExit(t *testing.T, fn func(context.Context, string, svc.State) (svc.State, error)) {
	t.Helper()
	prev := waitForServiceStartPendingExit
	waitForServiceStartPendingExit = fn
	t.Cleanup(func() {
		waitForServiceStartPendingExit = prev
	})
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func withProcmgrInstallRoot(t *testing.T, installRoot string, fn func()) {
	t.Helper()
	prev := procmgrInstallRootForDefinitionCheck
	procmgrInstallRootForDefinitionCheck = func() (string, error) {
		return installRoot, nil
	}
	t.Cleanup(func() {
		procmgrInstallRootForDefinitionCheck = prev
	})
	fn()
}

func writeProcmgrDefinitionFile(t *testing.T, fileName string) string {
	t.Helper()
	installRoot := t.TempDir()
	processesDir := filepath.Join(installRoot, "processes.d")
	require.NoError(t, os.MkdirAll(processesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(processesDir, fileName), []byte("description: test\n"), 0o644))
	return installRoot
}
