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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

func TestExecutorTelemetryLifecycle(t *testing.T) {
	for _, tt := range []struct {
		name       string
		enabled    bool
		listenFail bool
		idleExit   bool
	}{
		{name: "enabled", enabled: true},
		{name: "disabled"},
		{name: "failed listener", enabled: true, listenFail: true},
		{name: "idle shutdown", enabled: true, idleExit: true},
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
			hostname, _ := hostnamemock.NewMock("test-host")
			idle := make(chan struct{}, 1)
			runner := &PrivateActionRunner{
				coreConfig: cfg, hostnameGetter: hostname, logger: logmock.New(t), ipc: ipc,
				rcClient: telemetryTestRCClient{}, keysManager: telemetryTestKeysManager{},
				startChan:  make(chan struct{}),
				shutdowner: shutdownFunc(func() error { idle <- struct{}{}; return nil }),
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
