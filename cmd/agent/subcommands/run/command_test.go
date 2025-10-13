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
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		IoTCommands(newGlobalParamsTest(t)),
		[]string{"run"},
		StartCore,
		func(_ pidimpl.Params, _ core.BundleParams) {})
}

func TestCommandPidfile(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		IoTCommands(newGlobalParamsTest(t)),
		[]string{"run", "--pidfile", "/pid/file"},
		StartCore,
		func(pidParams pidimpl.Params, _ core.BundleParams) {
			require.Equal(t, "/pid/file", pidParams.PIDfilePath)
		})
}

func newGlobalParamsTest(t *testing.T) *command.GlobalParams {
	// Because getSharedFxOption uses fx.Invoke, demultiplexer component is built
	// which lead to build:
	//   - config.Component which requires a valid datadog.yaml
	//   - hostname.Component which requires a valid hostname
	config := path.Join(t.TempDir(), "datadog.yaml")
	err := os.WriteFile(config, []byte("hostname: test"), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: config,
	}
}
