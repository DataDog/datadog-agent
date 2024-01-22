// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'updater run'.
package run

import (
	"context"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	updaterrccomp "github.com/DataDog/datadog-agent/comp/updater/rc"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/spf13/cobra"
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
			}, run)
		},
	}
	return []*cobra.Command{runCmd}
}

func runFxWrapper(params *cliParams, fct interface{}) error {
	ctx := context.Background()
	return fxutil.Run(
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForDaemon("UPDATER", "updater.log_file", pkgconfig.DefaultUpdaterLogFile),
		}),
		core.Bundle(),
		fx.Supply(updatercomp.Options{
			Package: params.Package,
		}),
		updaterrccomp.Module(),
		updatercomp.Module(),
		localapi.Module(),
		fx.Invoke(fct),
	)
}

func run(updater *updater.Updater, localAPI *updater.LocalAPI) error {
	return localAPI.Serve()
}
