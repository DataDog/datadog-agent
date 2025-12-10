// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadfilterlist implements 'agent workloadfilter'.
package workloadfilterlist

import (
	"encoding/json"
	"fmt"
	"sort"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/fatih/color"
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
				secretsnoopfx.Module(),
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

	fmt.Fprintf(color.Output, "    %s\n\n", color.HiCyanString("=== Workload Filter Status ==="))

	// Container Autodiscovery Filters
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Container Autodiscovery Filters --------"))
	printFilter(fmt.Sprintf("  %-16s", "GlobalFilter:"), filterComponent.GetContainerAutodiscoveryFilters(workloadfilter.GlobalFilter))
	printFilter(fmt.Sprintf("  %-16s", "MetricsFilter:"), filterComponent.GetContainerAutodiscoveryFilters(workloadfilter.MetricsFilter))
	printFilter(fmt.Sprintf("  %-16s", "LogsFilter:"), filterComponent.GetContainerAutodiscoveryFilters(workloadfilter.LogsFilter))

	// Service Autodiscovery Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Service Autodiscovery Filters --------"))
	printFilter(fmt.Sprintf("  %-16s", "GlobalFilter:"), filterComponent.GetServiceAutodiscoveryFilters(workloadfilter.GlobalFilter))
	printFilter(fmt.Sprintf("  %-16s", "MetricsFilter:"), filterComponent.GetServiceAutodiscoveryFilters(workloadfilter.MetricsFilter))

	// Endpoint Autodiscovery Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Endpoint Autodiscovery Filters --------"))
	printFilter(fmt.Sprintf("  %-16s", "GlobalFilter:"), filterComponent.GetEndpointAutodiscoveryFilters(workloadfilter.GlobalFilter))
	printFilter(fmt.Sprintf("  %-16s", "MetricsFilter:"), filterComponent.GetEndpointAutodiscoveryFilters(workloadfilter.MetricsFilter))

	// Container Shared Metric Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Container Shared Metrics --------"))
	printFilter(fmt.Sprintf("  %-16s", "SharedMetrics:"), filterComponent.GetContainerSharedMetricFilters())

	// Container Paused Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Container Paused Filters --------"))
	printFilter(fmt.Sprintf("  %-16s", "PausedContainers:"), filterComponent.GetContainerPausedFilters())

	// Pod Shared Metric Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Pod Shared Metrics --------"))
	printFilter(fmt.Sprintf("  %-16s", "PodMetrics:"), filterComponent.GetPodSharedMetricFilters())

	// Container SBOM Filters
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Container SBOM Filters --------"))
	printFilter(fmt.Sprintf("  %-16s", "SBOM:"), filterComponent.GetContainerSBOMFilters())

	// Print raw filter configuration
	fmt.Fprintln(color.Output)
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("-------- Raw Filter Configuration --------"))

	configString, err := filterComponent.GetFilterConfigString()
	if err != nil {
		fmt.Fprintf(color.Output, "      %s\n", color.HiRedString("-> No filters configured"))
		fmt.Fprintf(color.Output, "         %s %s\n", color.HiRedString("raw config:"), color.RedString(configString))
		fmt.Fprintln(color.Output, color.HiCyanString("    ---------------------------------------------"))
		return nil
	}
	var filterConfig map[string]any
	if err := json.Unmarshal([]byte(configString), &filterConfig); err != nil {
		return fmt.Errorf("failed to unmarshal filter configuration: %w", err)
	}

	sortedKeys := make([]string, 0, len(filterConfig))
	for key := range filterConfig {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		value := filterConfig[key]
		display := fmt.Sprintf("%v", value)
		if display == "" || display == "[]" || display == "map[]" || display == "<nil>" {
			display = color.HiYellowString("not configured")
		}
		fmt.Fprintf(color.Output, "      %-28s %s\n", key+":", display)
	}
	fmt.Fprintf(color.Output, "    %s\n", color.HiCyanString("---------------------------------------------"))

	return nil
}

func printFilter(name string, bundle workloadfilter.FilterBundle) {
	if bundle == nil {
		fmt.Fprintf(color.Output, "%s: No filters configured\n", name)
		return
	}

	errors := bundle.GetErrors()
	if len(errors) > 0 {
		fmt.Fprintf(color.Output, "%s %s %s\n", color.HiRedString("✗"), name, color.HiRedString("failed to load"))
		for _, err := range errors {
			fmt.Fprintf(color.Output, "        Error: %s\n", err)
		}
		return
	}

	fmt.Fprintf(color.Output, "%s %s Loaded successfully\n", color.HiGreenString("✓"), name)
}
