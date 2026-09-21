// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package runexecutor

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnoop "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	secretsnoop "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunExecutorCommand(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		commands := Commands(newGlobalParamsTest(t, false))
		err := commands[0].RunE(nil, []string{"run-executor"})
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, true))
			return commands[0].RunE(nil, []string{"run-executor"})
		})
	})
}

func TestLaunchPathsOverrideConfiguration(t *testing.T) {
	dir := t.TempDir()
	confPath := filepath.Join(dir, "datadog.yaml")
	fleetDir := t.TempDir()
	extraPath := filepath.Join(dir, "extra.yaml")
	for _, file := range []string{confPath, extraPath, filepath.Join(fleetDir, "datadog.yaml")} {
		require.NoError(t, os.WriteFile(file, []byte(`
ipc_cert_file_path: /config/cert.pem
private_action_runner:
  executor:
    socket_path: /config/executor.sock
  task_concurrency: 9
`), 0600))
	}
	t.Setenv("DD_IPC_CERT_FILE_PATH", "/env/cert.pem")
	t.Setenv("DD_PRIVATE_ACTION_RUNNER_EXECUTOR_SOCKET_PATH", "/env/executor.sock")
	t.Setenv("DD_FLEET_POLICIES_DIR", fleetDir)
	params := &cliParams{
		GlobalParams:   &command.GlobalParams{ConfFilePath: confPath, ExtraConfFilePath: []string{extraPath}},
		executorSocket: filepath.Join(dir, "launch.sock"),
		ipcCertFile:    filepath.Join(dir, "launch.pem"),
	}
	cfg := fxutil.Test[config.Component](t,
		config.Module(), secretsnoop.Module(), delegatedauthnoop.Module(),
		fx.Supply(params.configParams()),
	)
	require.Equal(t, params.executorSocket, cfg.GetString(par.PARExecutorSocketPath))
	require.Equal(t, params.ipcCertFile, cfg.GetString("ipc_cert_file_path"))
	require.Equal(t, model.SourceCLI, cfg.GetSource(par.PARExecutorSocketPath))
	require.Equal(t, model.SourceCLI, cfg.GetSource("ipc_cert_file_path"))
	require.Equal(t, 9, cfg.GetInt("private_action_runner.task_concurrency"))
}

func TestEmptyLaunchFlags(t *testing.T) {
	for _, flag := range []string{"executor-socket", "ipc-cert-file"} {
		t.Run(flag, func(t *testing.T) {
			cmd := Commands(&command.GlobalParams{})[0]
			require.NoError(t, cmd.ParseFlags([]string{"--" + flag, ""}))
			require.ErrorContains(t, cmd.PreRunE(cmd, nil), "--"+flag+" must not be empty")
		})
	}
}

func TestLaunchFlags(t *testing.T) {
	cmd := Commands(&command.GlobalParams{})[0]
	require.NoError(t, cmd.ParseFlags([]string{"--executor-socket", "/launch.sock", "--ipc-cert-file", "/launch.pem"}))
	require.NoError(t, cmd.PreRunE(cmd, nil))
	socket, err := cmd.Flags().GetString("executor-socket")
	require.NoError(t, err)
	require.Equal(t, "/launch.sock", socket)
	cert, err := cmd.Flags().GetString("ipc-cert-file")
	require.NoError(t, err)
	require.Equal(t, "/launch.pem", cert)
}

func newGlobalParamsTest(t *testing.T, enabled bool) *command.GlobalParams {
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	configContent := `
hostname: test
private_action_runner:
  enabled: %v
  private_key: test_private_key
  urn: test_urn
api_key: test_key
`
	err := os.WriteFile(configPath, []byte(fmt.Sprintf(configContent, enabled)), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
