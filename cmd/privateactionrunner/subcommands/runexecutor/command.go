// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package runexecutor is the run-executor private-action-runner subcommand: it runs
// the on-demand Go executor of the split deployment model, serving the local
// control<->executor gRPC service instead of polling OPMS.
package runexecutor

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	settingsfx "github.com/DataDog/datadog-agent/comp/core/settings/fx"
	statsdfx "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/fx"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformfx "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/fx"
	eventplatformreceiverimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/impl"
	remotetraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	privateactionrunnerfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/fx"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	rcclientfx "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/fx"
	rcservicefx "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/fx"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
}

// runExecutor runs the private action runner in on-demand executor mode.
func runExecutor(ctx context.Context, confPath string, extraConfFiles []string) error {
	fxOptions := []fx.Option{
		fx.Provide(func() context.Context { return ctx }),
		fx.Invoke(func(shutdowner fx.Shutdowner) {
			go func() {
				<-ctx.Done()
				_ = shutdowner.Shutdown()
			}()
		}),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(confPath, config.WithExtraConfFiles(extraConfFiles)),
			LogParams:    log.ForDaemon(command.LoggerName, pkgconfigsetup.PARLogFile, defaultpaths.GetDefaultPrivateActionRunnerLogFile())}),
		core.Bundle(core.WithSecrets()),
		fx.Provide(func(c config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level": commonsettings.NewLogLevelRuntimeSetting(),
				},
				Config: c,
			}
		}),
		settingsfx.Module(),
		remotehostnameimpl.Module(),
		ipcfx.ModuleReadWrite(),
		rcservicefx.Module(),
		rcclientfx.Module(),
		fx.Supply(rcclient.Params{AgentName: "private-action-runner", AgentVersion: version.AgentVersion}),
		getTaggerModule(),
		remotetraceroute.Module(),
		logscompressionfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformfx.Module(eventplatform.NewDefaultParams()),
		statsdfx.Module(),
		privateactionrunnerfx.ExecutorModule(),
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

	runExecutorCmd := &cobra.Command{
		Use:   "run-executor",
		Short: "Run the Private Action Runner on-demand executor",
		Long:  `Runs the private-action-runner on-demand executor (split deployment model) in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runExecutor(context.Background(), globalParams.ConfFilePath, cliParams.ExtraConfFilePath)
		},
	}

	return []*cobra.Command{runExecutorCmd}
}
