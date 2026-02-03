// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package run is the run private-action-runner subcommand
package run

import (
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	parapp "github.com/DataDog/datadog-agent/pkg/privateactionrunner/app"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'private-action-runner' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Private Action Runner",
		Long:  `Runs the private-action-runner in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			err := fxutil.Run(
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath)),
					LogParams:    log.ForDaemon(command.LoggerName, "privateactionrunner.log_file", pkgconfigsetup.DefaultPrivateActionRunnerLogFile)}),
				core.Bundle(),
				secretsnoopfx.Module(),
				fx.Provide(func(c config.Component) settings.Params {
					return settings.Params{
						Settings: map[string]settings.RuntimeSetting{
							"log_level": commonsettings.NewLogLevelRuntimeSetting(),
						},
						Config: c,
					}
				}),
				settingsimpl.Module(),
				ipcfx.ModuleReadWrite(),
				rcserviceimpl.Module(),
				rcclientimpl.Module(),
				fx.Supply(rcclient.Params{AgentName: "private-action-runner", AgentVersion: version.AgentVersion}),
				fx.Invoke(runPrivateActionRunner),
			)
			if errors.Is(err, parapp.ErrNotEnabled) {
				return nil
			}
			return err
		},
	}

	return []*cobra.Command{runCmd}
}

// runPrivateActionRunner creates the private action runner app and registers lifecycle hooks
func runPrivateActionRunner(lc fx.Lifecycle, log log.Component, config config.Component, rcClient rcclient.Component) error {
	app, err := parapp.NewApp(config, pkgrcclient.NewAdapter(rcClient))
	if err != nil {
		return err
	}
	lc.Append(fx.Hook{OnStart: app.Start, OnStop: app.Stop})
	return nil
}
