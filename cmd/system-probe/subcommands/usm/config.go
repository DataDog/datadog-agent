// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package usm provides debugging and diagnostic commands for Universal Service Monitoring.
package usm

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	sysconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	fetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher/sysprobe"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// configParams holds CLI flags for the config command.
type configParams struct {
	*command.GlobalParams
	outputJSON bool
}

// makeConfigCommand returns the "usm config" cobra command.
func makeConfigCommand(globalParams *command.GlobalParams) *cobra.Command {
	params := &configParams{GlobalParams: globalParams}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show Universal Service Monitoring configuration",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runConfig,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams(""),
					SysprobeConfigParams: sysconfigimpl.NewParams(sysconfigimpl.WithSysProbeConfFilePath(params.ConfFilePath), sysconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot("SYS-PROBE", "off", false),
				}),
				core.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&params.outputJSON, "json", false, "Output configuration as JSON")

	return cmd
}

// runConfig is the main implementation of the config command.
func runConfig(sysprobeconfig sysconfigcomponent.Component, params *configParams) error {
	// Use the exact same logic as the config command - fetch from running system-probe
	runtimeConfig, err := fetcher.SystemProbeConfig(sysprobeconfig, nil)
	if err != nil {
		return err
	}

	// Parse the YAML config
	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(runtimeConfig), &fullConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Extract service_monitoring_config section
	usmConfig, ok := fullConfig["service_monitoring_config"].(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("service_monitoring_config not found in runtime config")
	}

	if params.outputJSON {
		return outputConfigJSON(usmConfig)
	}

	return outputConfigHumanReadable(usmConfig)
}

// outputConfigHumanReadable prints configuration in a text-based format.
func outputConfigHumanReadable(cfg map[interface{}]interface{}) error {
	// Convert YAML data to a formatted output
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	fmt.Println("service_monitoring_config:")
	// Print with indentation
	for _, line := range splitLines(string(yamlData)) {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

// outputConfigJSON encodes the configuration as indented JSON.
func outputConfigJSON(cfg map[interface{}]interface{}) error {
	// Convert map[interface{}]interface{} to map[string]interface{} for JSON encoding
	converted := convertToStringMap(cfg)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(converted)
}

// convertToStringMap converts map[interface{}]interface{} to map[string]interface{}
func convertToStringMap(m map[interface{}]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		key := fmt.Sprintf("%v", k)
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[key] = convertToStringMap(val)
		case []interface{}:
			result[key] = convertSlice(val)
		default:
			result[key] = v
		}
	}
	return result
}

// convertSlice converts slices that may contain map[interface{}]interface{}
func convertSlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[interface{}]interface{}:
			result[i] = convertToStringMap(val)
		case []interface{}:
			result[i] = convertSlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// splitLines splits a string by newlines
func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
