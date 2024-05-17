// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/localapi/localapiimpl"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/updater/updater/updaterimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func runCommand(global *command.GlobalParams) *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "run",
		Short:   "Runs the installer",
		GroupID: "daemon",
		Long:    ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runFxWrapper(global)
		},
	}
	return runCmd
}

func runFxWrapper(global *command.GlobalParams) error {
	ctx := context.Background()

	return fxutil.OneShot(
		run,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(global.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForDaemon("INSTALLER", "installer.log_file", pkgconfig.DefaultUpdaterLogFile),
		}),
		core.Bundle(),
		fx.Supply(&rcservice.Params{
			Options: []service.Option{
				service.WithDatabaseFileName("remote-config-installer.db"),
			},
		}),
		rctelemetryreporterimpl.Module(),
		rcserviceimpl.Module(),
		updaterimpl.Module(),
		localapiimpl.Module(),
		telemetryimpl.Module(),
		fx.Supply(pidimpl.NewParams(global.PIDFilePath)),
	)
}

func run(shutdowner fx.Shutdowner, _ pid.Component, _ localapi.Component, _ telemetry.Component) error {
	handleSignals(shutdowner)
	return nil
}

func handleSignals(shutdowner fx.Shutdowner) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("Received signal %d (%v)", signo, signo)
			_ = shutdowner.Shutdown()
			return
		}
	}
}
