// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage

// Package coverage implements 'otel-agent coverage' useful to compute code coverage in E2E tests.
package coverage

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// SetupCoverageCommand adds the coverage command to the otel-agent command tree.
func SetupCoverageCommand(globalParamsGetter func() *subcommands.GlobalParams, cmd *cobra.Command) {
	cmd.AddCommand(MakeCommand(globalParamsGetter))
}

// MakeCommand returns a command for the `coverage` CLI command
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "coverage",
		Short: "Handle coverage generation for a running otel-agent",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			globalParams := globalParamsGetter()
			return fxutil.OneShot(requestCoverage,
				fx.Supply(config.NewAgentParams(globalParams.CoreConfPath, config.WithExtraConfFiles(globalParams.ConfPaths))),
				logfx.Module(),
				fx.Supply(log.ForOneShot(globalParams.LoggerName, "off", true)),
				secretsnoopfx.Module(),
				delegatedauthnoopfx.Module(),
				config.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
		SilenceUsage: true,
	}
}

func requestCoverage(_ log.Component, _ config.Component, ipc ipc.Component) error {
	extensionURL := pkgconfigsetup.Datadog().GetString("otelcollector.extension_url")
	u, err := url.Parse(extensionURL)
	if err != nil {
		return fmt.Errorf("failed to parse extension URL %q: %w", extensionURL, err)
	}
	u.Path = "/coverage"

	resp, err := ipc.GetClient().Get(u.String())
	if err != nil {
		return err
	}

	fmt.Printf("Coverage request sent, response: %s\n", resp)
	return nil
}
