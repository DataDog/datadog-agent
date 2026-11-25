// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build ncm

package check

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/def"
	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	corechecks "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ncm "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkconfigmanagement"
	ncmstore "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/store"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestCommandNCM(t *testing.T) {
	testCommandNCM(t, false, false)
}

func TestCommandNCMLockedDatabase(t *testing.T) {
	testCommandNCM(t, true, true)
}

// With rollback enabled and no Agent holding the lock, the CLI must still leave the database alone.
func TestCommandNCMDoesNotOpenRollbackDatabase(t *testing.T) {
	testCommandNCM(t, true, false)
}

func testCommandNCM(t *testing.T, rollbackEnabled, lockDatabase bool) {
	t.Helper()
	corechecks.WithTestCatalog(t)
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "ncm_config.db")
	if lockDatabase {
		// Simulate a running Agent holding the rollback database open.
		store, err := ncmstore.Open(dbPath)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, store.Close(context.Background())) })
	}
	confFilePath := filepath.Join(tempDir, "datadog.yaml")
	conf := fmt.Sprintf("hostname: test\npython_lazy_loading: true\nrun_path: %q\nnetwork_devices:\n  config_management:\n    rollback:\n      enabled: %t\n", tempDir, rollbackEnabled)
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
		func(cliParams *cliParams, cfg config.Component, logger log.Component, demux demultiplexer.Component, ncmComp option.Option[networkconfigmanagement.Component]) {
			require.Equal(t, []string{ncm.CheckName}, cliParams.args)
			require.Equal(t, `.ip_address == "10.27.66.253"`, cliParams.instanceFilter)
			require.Equal(t, tempDir, cfg.GetString("run_path"))
			// The CLI's rollback override is scoped to the NCM component.
			require.Equal(t, rollbackEnabled, cfg.GetBool(ncmRollbackEnabledKey))
			_, ok := ncmComp.Get()
			require.True(t, ok)
			if !lockDatabase {
				require.NoFileExists(t, dbPath)
			}

			// Exercise run's registration path, stopping before autodiscovery starts.
			// Dependencies unused before that point can be nil.
			stopErr := errors.New("stop after registering checks")
			err := run(cfg, cliParams, logger, demux, nil, nil, nil,
				registrationAutodiscovery{err: stopErr}, nil, nil, nil,
				option.None[collector.Component](), nil, noopsimpl.NewComponent(),
				option.None[integrations.Component](), nil, nil, ncmComp, nil)
			require.ErrorIs(t, err, stopErr)

			loader, err := corechecks.NewGoCheckLoader()
			require.NoError(t, err)
			instance := integration.Data("ip_address: 10.27.66.253\nauth:\n  username: admin\n  password: test\n")
			// Catalog lookup and configuration must both work with the real component.
			loadedCheck, err := loader.Load(demux, integration.Config{
				Name:       ncm.CheckName,
				InitConfig: integration.Data("ssh:\n  insecure_skip_verify: true\n"),
				Instances:  []integration.Data{instance},
				Source:     "test",
				Provider:   "file",
			}, instance, 0)
			require.NoError(t, err)
			require.Equal(t, ncm.CheckName, loadedCheck.String())
		})
}

// registrationAutodiscovery stops run immediately after catalog registration.
type registrationAutodiscovery struct {
	autodiscovery.Component
	err error
}

func (a registrationAutodiscovery) LoadAndRun(context.Context) error {
	return a.err
}

func TestCommandOtherCheckDoesNotInitializeNCM(t *testing.T) {
	for _, checkName := range []string{"cpu", ""} {
		t.Run(checkName, func(t *testing.T) {
			tempDir := t.TempDir()
			confFilePath := filepath.Join(tempDir, "datadog.yaml")
			conf := fmt.Sprintf("hostname: test\nrun_path: %q\nnetwork_devices:\n  config_management:\n    rollback:\n      enabled: true\n", tempDir)
			require.NoError(t, os.WriteFile(confFilePath, []byte(conf), 0644))
			command := MakeCommand(func() GlobalParams {
				return GlobalParams{ConfFilePath: confFilePath}
			}, fx.Options())
			args := []string{"check"}
			if checkName != "" {
				args = append(args, checkName)
			}

			fxutil.TestOneShotSubcommand(t, []*cobra.Command{command}, args, run,
				func(cfg config.Component, ncmComp option.Option[networkconfigmanagement.Component]) {
					require.True(t, cfg.GetBool("network_devices.config_management.rollback.enabled"))
					_, ok := ncmComp.Get()
					require.False(t, ok, "unrelated checks must not initialize NCM")
					require.NoFileExists(t, filepath.Join(tempDir, "ncm_config.db"))
				})
		})
	}
}
