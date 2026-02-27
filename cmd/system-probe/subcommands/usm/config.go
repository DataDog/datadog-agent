// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package usm provides debugging and diagnostic commands for Universal Service Monitoring.
package usm

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

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
func runConfig(sysprobeconfig sysconfigcomponent.Component, _ *command.GlobalParams) error {
	// Fetch config from running system-probe
	runtimeConfig, err := fetcher.SystemProbeConfig(sysprobeconfig, nil)
	if err != nil {
		return err
	}

	// Parse the full config once
	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(runtimeConfig), &fullConfig); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Extract service_monitoring_config section
	usmConfig, ok := fullConfig["service_monitoring_config"]
	if !ok {
		return errors.New("service_monitoring_config not found in runtime config")
	}

	// Output as YAML
	if err := yaml.NewEncoder(os.Stdout).Encode(map[string]interface{}{
		"service_monitoring_config": usmConfig,
	}); err != nil {
		return fmt.Errorf("failed to format config: %w", err)
	}
	return nil
}
