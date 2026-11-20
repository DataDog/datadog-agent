// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package check

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ncm "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkconfigmanagement"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestCommandNCM(t *testing.T) {
	testCommandNCM(t, false)
}

func TestCommandNCMLockedDatabase(t *testing.T) {
	testCommandNCM(t, true)
}

func testCommandNCM(t *testing.T, lockDatabase bool) {
	t.Helper()
	tempDir := t.TempDir()
	if lockDatabase {
		// Simulate a running Agent holding the rollback database open.
		store, err := ncmstore.Open(filepath.Join(tempDir, "ncm_config.db"))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, store.Close(context.Background())) })
	}
	confFilePath := filepath.Join(tempDir, "datadog.yaml")
	conf := fmt.Sprintf("hostname: test\nrun_path: %q\nnetwork_devices:\n  config_management:\n    rollback:\n      enabled: %t\n", tempDir, lockDatabase)
	require.NoError(t, os.WriteFile(confFilePath, []byte(conf), 0644))

	command := MakeCommand(func() GlobalParams {
		return GlobalParams{ConfFilePath: confFilePath}
	}, fx.Decorate(func() demultiplexer.Component {
		// Keep the NCM component real without requiring a running Agent's IPC.
		return mocksender.CreateDefaultDemultiplexer(t)
	}))

	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{command},
		[]string{"check", ncm.CheckName, "--instance-filter", `.ip_address == "10.27.66.253"`},
		run,
		func(cliParams *cliParams, cfg config.Component, demux demultiplexer.Component, ncmComp option.Option[networkconfigmanagement.Component]) {
			require.Equal(t, []string{ncm.CheckName}, cliParams.args)
			require.Equal(t, `.ip_address == "10.27.66.253"`, cliParams.instanceFilter)
			require.Equal(t, tempDir, cfg.GetString("run_path"))
			require.Equal(t, lockDatabase, cfg.GetBool("network_devices.config_management.rollback.enabled"))

			checkFactory := ncm.Factory(cfg, ncmComp)
			factory, ok := checkFactory.Get()
			require.True(t, ok, "the check command must provide the NCM component")
			// Configuring the check registers the device with the real component.
			// A disabled or uninitialized component would return an error here.
			err := factory().Configure(demux, integration.FakeConfigHash,
				integration.Data("ip_address: 10.27.66.253\nauth:\n  username: admin\n  password: test\n"),
				integration.Data("ssh:\n  insecure_skip_verify: true\n"), "test", "file")
			require.NoError(t, err)
		})
}
