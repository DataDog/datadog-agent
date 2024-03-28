// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap implements 'updater bootstrap'.
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/updater"
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
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstraps the package with the first version.",
		Long: `Installs the first version of the package managed by this updater.
		This first version is sent remotely to the agent and can be configured from the UI.
		This command will exit after the first version is installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return boostrapFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
				pkg:          pkg,
			})
		},
	}
	bootstrapCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	bootstrapCmd.Flags().StringVarP(&pkg, "package", "P", "", "package to bootstrap")
	return []*cobra.Command{bootstrapCmd}
}

func boostrapFxWrapper(ctx context.Context, params *cliParams) error {
	return fxutil.OneShot(bootstrap,
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

func bootstrap(ctx context.Context, params *cliParams, config config.Component) error {
	err := updater.Bootstrap(ctx, params.pkg, config)
	if err != nil {
		return fmt.Errorf("could not install package: %w", err)
	}
	return nil
}
