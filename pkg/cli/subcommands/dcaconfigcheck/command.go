// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dcaconfigcheck builds a 'configcheck' command to be used in binaries.
package dcaconfigcheck

import (
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type GlobalParams struct {
	ConfFilePath string
}

type cliParams struct {
	verbose bool
}

// MakeCommand returns a `configcheck` command to be used by cluster-agent
// binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	cmd := &cobra.Command{
		Use:     "configcheck",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running cluster agent",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot("CLUSTER", "off", true),
				}),
				core.Bundle,
			)
		},
	}

	cmd.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")

	return cmd
}

func run(log log.Component, config config.Component, cliParams *cliParams) error {
	return flare.GetClusterAgentConfigCheck(color.Output, cliParams.verbose)
}
