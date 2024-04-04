// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'trace-agent config' cli.
package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/config/fetcher"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"go.uber.org/fx"

	"github.com/spf13/cobra"
)

// MakeCommand returns a command for the `config` CLI command
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running trace-agent",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			return fxutil.OneShot(printConfig,
				fx.Supply(config.NewAgentParams(globalParamsGetter().ConfPath)),
				fx.Provide(func() optional.Option[secrets.Component] { return optional.NewNoneOption[secrets.Component]() }),
				config.Module(),
			)
		},
		SilenceUsage: true,
	}
}

func printConfig(config config.Component) error {
	fullConfig, err := fetcher.TraceAgentConfig(config)
	if err != nil {
		return fmt.Errorf("error fetching trace-agent configuration: %s", err)
	}
	fmt.Print(fullConfig)
	return nil
}
