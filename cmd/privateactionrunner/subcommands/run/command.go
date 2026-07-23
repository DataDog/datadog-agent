// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package run is the run private-action-runner subcommand
package run

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	settingsfx "github.com/DataDog/datadog-agent/comp/core/settings/fx"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	parconstants "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams
}

func startTelemetryServer(lc fx.Lifecycle, cfg config.Component, logger log.Component, tel telemetry.Component) {
	if os.Getenv(parconstants.InternalEnableTelemetryEnvVar) != "true" {
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/telemetry", tel.Handler())

	addr := net.JoinHostPort(configutils.GetBindHost(cfg), strconv.Itoa(cfg.GetInt("metrics_port")))
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			listener, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("unable to start private action runner telemetry server on %s: %w", addr, err)
			}

			logger.Infof("Starting private action runner telemetry server at http://%s/telemetry", listener.Addr().String())
			go func() {
				if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					logger.Warnf("Private action runner telemetry server stopped unexpectedly: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
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
		fx.Invoke(startTelemetryServer),
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
