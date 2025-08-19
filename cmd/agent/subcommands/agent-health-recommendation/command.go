// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealthrecommendation implements 'agent agent-health-recommendation'.
package agenthealthrecommendation

import (
	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthplatformfx "github.com/DataDog/datadog-agent/comp/core/health-platform/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	verbose     bool
	jsonOutput  bool
	severity    string
	location    string
	integration string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "agent-health-recommendation",
		Short: "Run health checks from all subcomponents and display issues found",
		Long: `agent-health-recommendation is a CLI tool that runs health checks from all 
subcomponents of the Datadog Agent health platform and displays the issues found.

This tool helps identify potential problems with the agent's health and provides
recommendations for improvement.`,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(runHealthRecommendation,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(command.ConfigName), config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "off", true)}),
				core.Bundle(),
				healthplatformfx.Module(),
			)
		},
	}

	// Add flags
	cmd.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVarP(&cliParams.jsonOutput, "json", "j", false, "Output results in JSON format")
	cmd.Flags().StringVarP(&cliParams.severity, "severity", "s", "", "Filter issues by severity (low, medium, high, critical)")
	cmd.Flags().StringVarP(&cliParams.location, "location", "l", "", "Filter issues by location (core-agent, log-agent, process-agent, etc.)")
	cmd.Flags().StringVarP(&cliParams.integration, "integration", "i", "", "Filter issues by integration/feature (logs, metrics, apm, etc.)")

	return []*cobra.Command{cmd}
}
