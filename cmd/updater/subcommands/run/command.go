// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'updater run'.
package run

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
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

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/localapi/localapiimpl"
	"github.com/DataDog/datadog-agent/comp/updater/updater/updaterimpl"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
)

type cliParams struct {
	command.GlobalParams
}

// Commands returns the run command
func Commands(global *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the updater",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFxWrapper(&cliParams{
				GlobalParams: *global,
			})
		},
	}
	return []*cobra.Command{runCmd}
}

func runFxWrapper(params *cliParams) error {
	ctx := context.Background()

	return fxutil.OneShot(
		run,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForDaemon("UPDATER", "updater.log_file", pkgconfig.DefaultUpdaterLogFile),
		}),
		core.Bundle(),
		fx.Supply(&rcservice.Params{
			Options: []service.Option{
				service.WithDatabaseFileName("remote-config-updater.db"),
			},
		}),
		rctelemetryreporterimpl.Module(),
		rcserviceimpl.Module(),
		updaterimpl.Module(),
		localapiimpl.Module(),
		fx.Supply(pidimpl.NewParams(params.PIDFilePath)),
	)
}

func run(_ pid.Component, _ localapi.Component) error {
	select {}
}
