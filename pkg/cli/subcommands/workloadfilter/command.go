// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterlist implements 'agent workloadfilter'.
package workloadfilterlist

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams
}

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath         string
	ExtraConfFilePaths   []string
	ConfigName           string
	LoggerName           string
	FleetPoliciesDirPath string
}

// MakeCommand returns a `workloadfilter` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	parentCmd := &cobra.Command{
		Use:   "workloadfilter",
		Short: "Print the workload filter status of a running agent",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalParamsGetter()
			cliParams.GlobalParams = globalParams

			return fxutil.OneShot(workloadFilterList,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(
						globalParams.ConfFilePath,
						config.WithConfigName(globalParams.ConfigName),
						config.WithExtraConfFiles(globalParams.ExtraConfFilePaths),
						config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
					),
					LogParams: log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),
				workloadfilterfx.Module(),
			)
		},
	}

	// Add verify-cel subcommand
	verifyCelCmd := &cobra.Command{
		Use:   "verify-cel",
		Short: "Validate CEL workload filter rules from a YAML file",
		Long:  ``,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return verifyCELConfig(cmd.OutOrStdout(), cmd.InOrStdin())
		},
	}

	parentCmd.AddCommand(verifyCelCmd)

	return parentCmd
}

func workloadFilterList(_ log.Component, filterComponent workloadfilter.Component, _ *cliParams) error {
	fmt.Println(filterComponent.String(true))
	return nil
}
