// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package usm provides debugging and diagnostic commands for Universal Service Monitoring.
package usm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysconfigcomponent "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	fetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher/sysprobe"
)

// makeConfigCommand returns the "usm config" cobra command.
func makeConfigCommand(globalParams *command.GlobalParams) *cobra.Command {
	return makeOneShotCommand(
		globalParams,
		"config",
		"Show Universal Service Monitoring configuration",
		runConfig,
	)
}

// runConfig is the main implementation of the config command.
func runConfig(sysprobeconfig sysconfigcomponent.Component, params *cmdParams) error {
	// Fetch config from running system-probe (already formatted as YAML string)
	runtimeConfig, err := fetcher.SystemProbeConfig(sysprobeconfig, nil)
	if err != nil {
		return err
	}

	// Extract just the service_monitoring_config section
	usmConfigYAML, err := extractUSMConfig(runtimeConfig)
	if err != nil {
		return err
	}

	if params.outputJSON {
		// Parse YAML and convert directly to JSON using yaml.v3 which handles types better
		var config interface{}
		if err := yaml.Unmarshal([]byte(usmConfigYAML), &config); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
		return outputJSON(config)
	}

	// For YAML output, just print the extracted section directly
	fmt.Print(usmConfigYAML)
	return nil
}

// extractUSMConfig extracts the service_monitoring_config section from the full config YAML
func extractUSMConfig(fullConfigYAML string) (string, error) {
	lines := strings.Split(fullConfigYAML, "\n")
	var usmLines []string
	inUSMSection := false
	baseIndent := ""

	for _, line := range lines {
		// Check if we're starting the service_monitoring_config section
		if strings.HasPrefix(strings.TrimSpace(line), "service_monitoring_config:") {
			inUSMSection = true
			// Capture the indentation level
			baseIndent = line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			usmLines = append(usmLines, line)
			continue
		}

		if inUSMSection {
			// Check if we've exited the section (line at same or lower indentation level)
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed != "" && !strings.HasPrefix(line, baseIndent+" ") && !strings.HasPrefix(line, baseIndent+"\t") {
				break
			}
			usmLines = append(usmLines, line)
		}
	}

	if len(usmLines) == 0 {
		return "", errors.New("service_monitoring_config not found in runtime config")
	}

	return strings.Join(usmLines, "\n") + "\n", nil
}
