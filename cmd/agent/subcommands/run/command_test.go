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
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommand(t *testing.T) {
	// FIXME: we stop AC to avoid a race condition in the test suite
	// Calling `Command` starts the AC, and it is currently not stopped
	// Not stopping it causes a race when running the other test of this package
	t.Cleanup(func() {
		if common.AC != nil {
			common.AC.Stop()
		}
	})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run"},
		run,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, true, secretParams.Enabled)
		})
	workloadmeta.SetGlobalStore(nil)
}

func TestCommandPidfile(t *testing.T) {
	// FIXME: we stop AC to avoid a race condition in the test suite
	// Calling `Command` starts the AC, and it is currently not stopped
	// Not stopping it causes a race when running the other test of this package
	t.Cleanup(func() {
		if common.AC != nil {
			common.AC.Stop()
		}
	})

	fxutil.TestOneShotSubcommand(t,
		Commands(newGlobalParamsTest(t)),
		[]string{"run", "--pidfile", "/pid/file"},
		run,
		func(cliParams *cliParams, coreParams core.BundleParams, secretParams secrets.Params) {
			require.Equal(t, "/pid/file", cliParams.pidfilePath)
			require.Equal(t, true, secretParams.Enabled)
		})
	workloadmeta.SetGlobalStore(nil)
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
