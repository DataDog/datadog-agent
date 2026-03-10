// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(_ pidimpl.Params, _ core.BundleParams) {})
}

func TestCommandPidfile(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run", "--pidfile", "/pid/file"},
		run,
		func(pidParams pidimpl.Params, _ core.BundleParams) {
			require.Equal(t, "/pid/file", pidParams.PIDfilePath)
		})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because getSharedFxOption uses fx.Invoke, demultiplexer component is built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	//   - hostname.Component which requires a valid hostname
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(configPath, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}

func TestValidateRemoteAgentConfigStream(t *testing.T) {
	t.Run("error when use_configstream true and registry enabled false", func(t *testing.T) {
		cfg := config.NewMock(t)
		cfg.SetWithoutSource("remote_agent.configstream.enabled", true)
		cfg.SetWithoutSource("remote_agent.registry.enabled", false)
		err := validateRemoteAgentConfigStream(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "remote_agent.configstream.enabled is true but remote_agent.registry.enabled is not")
	})

	t.Run("no error when use_configstream false", func(t *testing.T) {
		cfg := config.NewMock(t)
		cfg.SetWithoutSource("remote_agent.configstream.enabled", false)
		cfg.SetWithoutSource("remote_agent.registry.enabled", false)
		err := validateRemoteAgentConfigStream(cfg)
		require.NoError(t, err)
	})

	t.Run("no error when both use_configstream and registry enabled true", func(t *testing.T) {
		cfg := config.NewMock(t)
		cfg.SetWithoutSource("remote_agent.configstream.enabled", true)
		cfg.SetWithoutSource("remote_agent.registry.enabled", true)
		err := validateRemoteAgentConfigStream(cfg)
		require.NoError(t, err)
	})
}
