// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package run is the run private-action-runner subcommand
package run

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
}

// runPrivateActionRunner runs the private action runner with the given configuration and context.
// This function is shared between the CLI run command and the Windows service.
func runPrivateActionRunner(ctx context.Context, confPath string, extraConfFiles []string) error {
	fxOptions := []fx.Option{
		// Provide context for cancellation (Windows service uses this for graceful shutdown)
		fx.Provide(func() context.Context { return ctx }),
		// Setup shutdown listener for context cancellation (e.g., from Windows SCM)
		fx.Invoke(func(shutdowner fx.Shutdowner) {
			go func() {
				<-ctx.Done()
				_ = shutdowner.Shutdown()
			}()
		}),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(confPath, config.WithExtraConfFiles(extraConfFiles)),
			LogParams:    log.ForDaemon(command.LoggerName, pkgconfigsetup.PARLogFile, pkgconfigsetup.DefaultPrivateActionRunnerLogFile)}),
		core.Bundle(false),
		fx.Provide(func(c config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level": commonsettings.NewLogLevelRuntimeSetting(),
				},
				Config: c,
			}
		}),
		settingsimpl.Module(),
		remotehostnameimpl.Module(),
		ipcfx.ModuleReadWrite(),
		rcserviceimpl.Module(),
		rcclientimpl.Module(),
		fx.Supply(rcclient.Params{AgentName: "private-action-runner", AgentVersion: version.AgentVersion}),
		getTaggerModule(),
		remotetraceroute.Module(),
		logscompressionfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		privateactionrunnerfx.Module(),
	}

	err := fxutil.Run(fxOptions...)
	if errors.Is(err, privateactionrunner.ErrNotEnabled) {
		return nil
	}
	return err
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
			return runPrivateActionRunner(context.Background(), globalParams.ConfFilePath, cliParams.ExtraConfFilePath)
		},
	}

	return []*cobra.Command{runCmd}
}
