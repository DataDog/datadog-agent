// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setupunits

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type cliParams struct {
	command.GlobalParams
	pkg string
}

// Commands returns the bootstrap command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var timeout time.Duration
	var pkg string
	setupUnitsCmd := &cobra.Command{
		Use:   "setup-units",
		Short: "setup-units copies systemd units in the correct directory and loads/starts them",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return setupUnitsFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
				pkg:          pkg,
			})
		},
	}
	setupUnitsCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to setup-units with")
	setupUnitsCmd.Flags().StringVarP(&pkg, "package", "P", "", "Package name to setup units for")
	return []*cobra.Command{setupUnitsCmd}
}

func setupUnitsFxWrapper(ctx context.Context, params *cliParams) error {
	return fxutil.OneShot(setupUnits,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForOneShot("UPDATER", "info", true),
		}),
		core.Bundle(),
	)
}

func setupUnits(ctx context.Context, params *cliParams, config config.Component) error {
	return installer.SetupUnits(ctx, params.pkg, config)
}
