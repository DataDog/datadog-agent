// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage

// Package coverage implements 'trace-agent coverage' useful to compute code coverage in E2E tests.
package coverage

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// SetupCoverageCommand adds the coverage command to the trace-agent command tree.
func SetupCoverageCommand(globalParamsGetter func() *subcommands.GlobalParams, cmd *cobra.Command) {
	cmd.AddCommand(MakeCommand(globalParamsGetter))
}

// MakeCommand returns a command for the `config` CLI command
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "coverage",
		Short: "Handle coverage generation for a running trace-agent",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			return fxutil.OneShot(requestCoverage,
				fx.Supply(config.NewAgentParams(globalParamsGetter().ConfPath, config.WithFleetPoliciesDirPath(globalParamsGetter().FleetPoliciesDirPath))),
				logfx.Module(),
				fx.Supply(log.ForOneShot(globalParamsGetter().LoggerName, "off", true)),
				fx.Supply(option.None[secrets.Component]()),
				config.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
		SilenceUsage: true,
	}
}

func requestCoverage(_ log.Component, config config.Component, ipc ipc.Component) error {
	url := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("localhost:%v", pkgconfigsetup.Datadog().GetInt("apm_config.debug.port")),
		Path:   "/coverage",
	}
	resp, err := ipc.GetClient().Get(url.String())
	if err != nil {
		return err
	}

	fmt.Printf("Coverage request sent, response: %s\n", resp)
	return nil
}
