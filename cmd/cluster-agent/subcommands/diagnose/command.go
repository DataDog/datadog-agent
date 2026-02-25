// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package diagnose implements 'cluster-agent diagnose'.
package diagnose

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	include []string
}

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Execute some connectivity diagnosis on your system",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(core.WithSecrets(), core.WithDelegatedAuth()),
				diagnosefx.Module(),
			)
		},
	}

	cmd.Flags().StringSliceVar(&cliParams.include, "include", nil, "Comma-separated list of diagnosis to run")

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(CINT) Fix revive linter
func run(cfg config.Component, diagnoseComponent diagnose.Component, cliParams *cliParams) error {
	// Register both suites for diagnose subcommand
	catalog := diagnose.GetCatalog()
	catalog.Register(diagnose.AutodiscoveryConnectivity, func(_ diagnose.Config) []diagnose.Diagnosis {
		return connectivity.DiagnoseMetadataAutodiscoveryConnectivity()
	})
	catalog.Register(diagnose.CoreEndpointsConnectivity, func(_ diagnose.Config) []diagnose.Diagnosis {
		return connectivity.Diagnose(diagnose.Config{}, nil)
	})

	config := diagnose.Config{}
	suites := diagnose.Suites{}
	if len(cliParams.include) == 0 {
		if fn, ok := catalog.Suites[diagnose.AutodiscoveryConnectivity]; ok {
			suites[diagnose.AutodiscoveryConnectivity] = fn
		}
	} else {
		for _, name := range cliParams.include {
			if fn, ok := catalog.Suites[name]; ok {
				suites[name] = fn
			}
		}
	}
	if len(suites) == 0 {
		return format.Text(color.Output, config, &diagnose.Result{
			Runs: []diagnose.Diagnoses{
				{
					Name: "Diagnose",
					Diagnoses: []diagnose.Diagnosis{
						{
							Status:    diagnose.DiagnosisFail,
							Name:      "Diagnose",
							Category:  "All",
							Diagnosis: "No diagnose suite were found",
						},
					},
				},
			},
		})
	}

	result, err := diagnoseComponent.RunLocalSuite(suites, config)
	if err != nil {
		return err
	}

	return format.Text(color.Output, config, result)
}
