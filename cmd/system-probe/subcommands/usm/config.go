// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package usm provides debugging and diagnostic commands for Universal Service Monitoring.
package usm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

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
	usmConfig, ok := fullConfig["service_monitoring_config"]
	if !ok {
		return errors.New("service_monitoring_config not found in runtime config")
	}

	if params.outputJSON {
		return outputConfigJSON(usmConfig)
	}

	return outputConfigYAML(usmConfig)
}

// outputConfigYAML prints configuration in YAML format.
func outputConfigYAML(cfg interface{}) error {
	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	fmt.Println("service_monitoring_config:")
	// Print with indentation
	for _, line := range strings.Split(string(yamlData), "\n") {
		if line != "" {
			fmt.Printf("  %s\n", line)
		}
	}

	return nil
}

// outputConfigJSON encodes the configuration as indented JSON.
func outputConfigJSON(cfg interface{}) error {
	// Convert to JSON-compatible structure (yaml.v2 creates map[interface{}]interface{})
	jsonCompatible := convertToJSONCompatible(cfg)

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonCompatible)
}

// convertToJSONCompatible converts map[interface{}]interface{} to map[string]interface{}
// recursively so it can be JSON encoded.
func convertToJSONCompatible(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m := map[string]interface{}{}
		for k, v := range x {
			m[fmt.Sprint(k)] = convertToJSONCompatible(v)
		}
		return m
	case []interface{}:
		for i, v := range x {
			x[i] = convertToJSONCompatible(v)
		}
	}
	return i
}
