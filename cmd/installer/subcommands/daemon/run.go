// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pidimpl "github.com/DataDog/datadog-agent/comp/core/pid/impl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/comp/updater/localapi/localapiimpl"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/updater/updater/updaterimpl"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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

func getCommonFxOption(global *command.GlobalParams) fx.Option {
	ctx := context.Background()
	return fx.Options(fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(global.ConfFilePath),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            log.ForDaemon("INSTALLER", "installer.log_file", pkgconfigsetup.DefaultUpdaterLogFile),
		}),
		core.Bundle(core.WithSecrets()),
		hostnameimpl.Module(),
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
		ipcfx.ModuleReadWrite(),
	)
}
