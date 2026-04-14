// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package run is the "run" subcommand for par-executor.
package run

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/par-executor/command"
	"github.com/DataDog/datadog-agent/comp/core"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	remotehostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	rcservicefx "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/fx"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	executorimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/executor/impl"
	executorfx "github.com/DataDog/datadog-agent/comp/privateactionrunner/executor/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Commands returns the cobra "run" subcommand for par-executor.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the PAR executor (execution plane)",
		RunE: func(_ *cobra.Command, _ []string) error {
			if globalParams.SocketPath == "" {
				return fmt.Errorf("--socket is required")
			}
			return runParExecutor(context.Background(), globalParams.ConfFilePath, globalParams.ExtraConfFilePath, globalParams.SocketPath, globalParams.IdleTimeoutSeconds)
		},
	}
	return []*cobra.Command{runCmd}
}

func runParExecutor(ctx context.Context, cfgPath string, extraConfFiles []string, socketPath string, idleTimeoutSeconds int) error {
	return fxutil.Run(
		fx.Provide(func() context.Context { return ctx }),
		fx.Invoke(func(shutdowner fx.Shutdowner) {
			go func() {
				<-ctx.Done()
				_ = shutdowner.Shutdown()
			}()
		}),

		// === Identical to cmd/privateactionrunner/subcommands/run/command.go ===
		fx.Supply(core.BundleParams{
			ConfigParams: coreconfig.NewAgentParams(cfgPath, coreconfig.WithExtraConfFiles(extraConfFiles)),
			LogParams:    log.ForDaemon(command.LoggerName, pkgconfigsetup.PARLogFile, pkgconfigsetup.DefaultPrivateActionRunnerLogFile),
		}),
		core.Bundle(core.WithSecrets()),
		fx.Provide(func(c coreconfig.Component) settings.Params {
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
		rcservicefx.Module(),
		rcclientimpl.Module(),
		fx.Supply(rcclient.Params{AgentName: "par-executor", AgentVersion: version.AgentVersion}),
		// === End identical section ===

		// Removed vs. PAR: remotetraceroute, logscompression, eventplatform, tagger
		// (none of those are needed for the remote-actions bundle)

		// Pass CLI args into the executor component.
		fx.Supply(executorimpl.Params{
			SocketPath:         socketPath,
			IdleTimeoutSeconds: idleTimeoutSeconds,
		}),

		// Swap privateactionrunnerfx.Module() for the executor module.
		executorfx.Module(),
	)
}
