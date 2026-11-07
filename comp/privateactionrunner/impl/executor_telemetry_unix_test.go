// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && (linux || darwin)

package privateactionrunnerimpl

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func TestExecutorTelemetryLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name         string
		enabled      bool
		listenFail   bool
		idleExit     bool
		blockStartup string
	}{
		{name: "enabled", enabled: true},
		{name: "disabled"},
		{name: "failed listener", enabled: true, listenFail: true},
		{name: "idle shutdown", enabled: true, idleExit: true},
		{name: "stop during identity lookup", enabled: true, blockStartup: "identity"},
		{name: "stop during key manager startup", enabled: true, blockStartup: "keys"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Keep socket paths short enough for macOS's Unix socket limit.
			dir, err := os.MkdirTemp("", "par")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			socket := filepath.Join(dir, "e.sock")
			if tt.listenFail {
				socket = filepath.Join(dir, "missing", "e.sock")
			}
			key, _, err := util.GenerateKeys()
			require.NoError(t, err)
			encoded, err := key.MarshalJSON()
			require.NoError(t, err)
			ipc := ipcmock.New(t)
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				// GovCloud suppresses HTTP sends; these tests exercise ownership, not delivery.
				"site":                                       "ddog-gov.com",
				"private_action_runner.enabled":              tt.enabled,
				"private_action_runner.split_enabled":        true,
				"private_action_runner.self_enroll":          false,
				"private_action_runner.urn":                  util.MakeRunnerURN("us1", 42, "test-runner"),
				"private_action_runner.private_key":          base64.RawURLEncoding.EncodeToString(encoded),
				"private_action_runner.identity_file_path":   filepath.Join(dir, "identity.json"),
				"private_action_runner.executor.socket_path": socket,
				"private_action_runner.idle_timeout_seconds": 1,
			})
			t.Setenv("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED", "true")
			_, hostname := hostnamemock.NewMock("test-host")
			idle := make(chan struct{}, 1)
			runner := &PrivateActionRunner{
				coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t), ipc: ipc,
				identityStorage: enrollment.NewIdentityStorage(cfg, nil),
				rcClient:        telemetryTestRCClient{}, keysManager: telemetryTestKeysManager{},
				startChan:  make(chan struct{}),
				shutdowner: shutdownFunc(func() error { idle <- struct{}{}; return nil }),
			}
			if tt.blockStartup != "" {
				entered := make(chan context.Context, 1)
				release := make(chan struct{})
				unblock := sync.OnceFunc(func() { close(release) })
				t.Cleanup(unblock)
				block := func(ctx context.Context) {
					entered <- ctx
					<-release // Model a dependency that ignores cancellation until it returns.
				}
				if tt.blockStartup == "identity" {
					runner.hostnameGetter = blockingExecutorHostname{Mock: hostname, block: block}
				} else {
					runner.keysManager = blockingExecutorKeysManager{block: block}
				}
				metrics := &recordingMetricsClient{}
				runner.metricsClient = metrics
				runner.ownsMetricsClient = true
				started := make(chan error, 1)
				go func() { started <- runner.StartExecutor(context.Background()) }()
				var startupCtx context.Context
				select {
				case startupCtx = <-entered:
				case <-time.After(5 * time.Second):
					t.Fatal("executor did not enter startup")
				}
				stopCtx, cancelStop := context.WithCancel(context.Background())
				cancelStop()
				assert.ErrorIs(t, runner.StopExecutor(stopCtx), context.Canceled)
				assert.ErrorIs(t, startupCtx.Err(), context.Canceled)
				assert.Zero(t, metrics.flushCalls, "startup still owns the metrics client")
				assert.Zero(t, metrics.closeCalls, "startup still owns the metrics client")
				unblock()
				select {
				case err := <-started:
					assert.ErrorIs(t, err, context.Canceled)
				case <-time.After(5 * time.Second):
					t.Fatal("cancelled executor startup did not return")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, runner.StopExecutor(ctx))
				require.NoError(t, runner.StopExecutor(ctx))
				assert.Equal(t, 1, metrics.flushCalls)
				assert.Equal(t, 1, metrics.closeCalls)
				assert.Nil(t, runner.telemetry)
				assert.Nil(t, runner.executorDone, "cancelled startup must not launch the server")
				return
			}
			err = runner.StartExecutor(context.Background())
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				require.NoError(t, runner.StopExecutor(ctx))
			})
			if tt.listenFail {
				require.ErrorContains(t, err, "failed to listen on executor socket")
				require.Nil(t, runner.telemetry)
				return
			}
			require.NoError(t, err)
			if !tt.enabled {
				require.Nil(t, runner.telemetry)
				return
			}
			require.NotNil(t, runner.telemetry)
			if tt.idleExit {
				select {
				case <-idle:
				case <-time.After(5 * time.Second):
					t.Fatal("executor did not request idle shutdown")
				}
			}
		})
	}
}

type blockingExecutorHostname struct {
	hostnamemock.Mock
	block func(context.Context)
}

func (h blockingExecutorHostname) Get(ctx context.Context) (string, error) {
	h.block(ctx)
	return h.Mock.Get(ctx)
}

type blockingExecutorKeysManager struct {
	telemetryTestKeysManager
	block func(context.Context)
}

func (k blockingExecutorKeysManager) Start(ctx context.Context) {
	k.block(ctx)
}
