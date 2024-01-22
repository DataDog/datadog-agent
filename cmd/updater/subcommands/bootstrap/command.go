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
	"go.uber.org/fx"

	updaterrccomp "github.com/DataDog/datadog-agent/comp/updater/rc"

	"github.com/spf13/cobra"
)

type cliParams struct {
	command.GlobalParams
}

// Commands returns the bootstrap command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var timeout time.Duration
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstraps the package with the first version.",
		Long: `Installs the first version of the package managed by this updater.
		This first version is sent remotely to the agent and can be configured from the UI.
		This command will exit after the first version is installed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, _ := context.WithTimeout(context.Background(), timeout)
			return boostrapFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
			}, bootstrap)
		},
	}
	bootstrapCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	return []*cobra.Command{bootstrapCmd}
}

func boostrapFxWrapper(ctx context.Context, params *cliParams, fct interface{}) error {
	return fxutil.OneShot(fct,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForOneShot("UPDATER", "info", true),
		}),
		core.Bundle(),
		updaterrccomp.Module(),
	)
}

func bootstrap(ctx context.Context, params *cliParams, rc *updater.RemoteConfig) error {
	err := updater.Install(ctx, rc, params.Package)
	if err != nil {
		return fmt.Errorf("could not install package: %w", err)
	}
	return nil
}
