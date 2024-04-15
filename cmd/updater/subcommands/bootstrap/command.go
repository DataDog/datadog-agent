// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bootstrap implements 'installer bootstrap'.
package bootstrap

import (
	"context"
	"fmt"
	"strings"
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
	url     string
	pkg     string
	version string
}

// Commands returns the bootstrap command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var timeout time.Duration
	var url string
	var pkg string
	var version string
	bootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstraps the package with the first version.",
		Long: `Installs the first version of the package managed by this updater.
		This first version is sent remotely to the agent and can be configured from the UI.
		This command will exit after the first version is installed.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return bootstrapFxWrapper(ctx, &cliParams{
				GlobalParams: *global,
				url:          url,
				pkg:          pkg,
				version:      version,
			})
		},
	}
	bootstrapCmd.Flags().DurationVarP(&timeout, "timeout", "T", 3*time.Minute, "timeout to bootstrap with")
	bootstrapCmd.Flags().StringVarP(&url, "url", "u", "", "URL to fetch the package from")
	bootstrapCmd.Flags().StringVarP(&pkg, "package", "P", "", "Package name to bootstrap")
	bootstrapCmd.Flags().StringVarP(&version, "version", "V", "latest", "Version to bootstrap")
	return []*cobra.Command{bootstrapCmd}
}

func bootstrapFxWrapper(ctx context.Context, params *cliParams) error {
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
	if params.pkg == "" && params.url == "" {
		return installer.Bootstrap(ctx, config)
	}
	url := packageURL(config.GetString("site"), params.pkg, params.version)
	if params.url != "" {
		url = params.url
	}
	return installer.BootstrapURL(ctx, url, config)
}

func packageURL(site string, pkg string, version string) string {
	if site == "datad0g.com" {
		return stagingPackageURL(pkg, version)
	}
	return prodPackageURL(pkg, version)
}

func stagingPackageURL(pkg string, version string) string {
	return fmt.Sprintf("oci://docker.io/datadog/%s-package-dev:%s", strings.TrimPrefix(pkg, "datadog-"), version)
}

func prodPackageURL(pkg string, version string) string {
	return fmt.Sprintf("oci://public.ecr.aws/datadoghq/%s-package:%s", strings.TrimPrefix(pkg, "datadog-"), version)
}
