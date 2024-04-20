// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package purge implements 'installer purge'.
package purge

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	command.GlobalParams
	pkg string
}

// Commands returns the run command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var pkg string
	purgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge installer packages",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			return purgeFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
				pkg:          pkg,
			})
		},
	}
	purgeCmd.Flags().StringVarP(&pkg, "package", "P", "", "Package name to purge")
	return []*cobra.Command{purgeCmd}
}

func purgeFxWrapper(ctx context.Context, params *cliParams) error {
	return fxutil.OneShot(purge,
		fx.Provide(func() context.Context { return ctx }),
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForOneShot("INSTALLER", "info", true),
		}),
		core.Bundle(),
		telemetryimpl.Module(),
	)
}

func purge(ctx context.Context, params *cliParams, _ log.Component, _ telemetry.Component) (err error) {
	span, ctx := tracer.StartSpanFromContext(ctx, "cmd_purge")
	defer func() { span.Finish(tracer.WithError(err)) }()

	span.SetTag("params.pkg", params.pkg)

	if params.pkg != "" {
		return installer.PurgePackage(ctx, params.pkg)
	}
	return installer.Purge(ctx)
}
