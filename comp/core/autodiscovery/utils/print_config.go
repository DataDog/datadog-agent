// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils provides utility functions for the autodiscovery component.
package utils

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// PrintConfig prints a human-readable representation of a configuration with any secrets scrubbed.
func PrintConfig(w io.Writer, config integration.Config, checkName string) {
	if checkName != "" && config.Name != checkName {
		return
	}
	configDigest := config.FastDigest()
	if !config.ClusterCheck {
		fmt.Fprintf(w, "\n=== %s check ===\n", color.GreenString(config.Name))
	} else {
		fmt.Fprintf(w, "\n=== %s cluster check ===\n", color.GreenString(config.Name))
	}

	if config.Provider != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.CyanString(config.Provider))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.RedString("Unknown provider"))
	}
	if config.Source != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.CyanString(config.Source))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.RedString("Unknown configuration source"))
	}
	for _, inst := range config.Instances {
		ID := string(checkid.BuildID(config.Name, configDigest, inst, config.InitConfig))
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Config for instance ID"), color.CyanString(ID))
		printScrubbed(w, inst)
		fmt.Fprintln(w, "~")
	}
	if len(config.InitConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Init Config"))
		printScrubbed(w, config.InitConfig)
	}
	if len(config.MetricConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Metric Config"))
		printScrubbed(w, config.MetricConfig)
	}
	if len(config.LogsConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Log Config"))
		printScrubbed(w, config.LogsConfig)
	}
	if config.IsTemplate() {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Auto-discovery IDs"))
		for _, id := range config.ADIdentifiers {
			fmt.Fprintf(w, "* %s\n", color.CyanString(id))
		}
		printContainerExclusionRulesInfo(w, config)
	}
	if config.NodeName != "" {
		state := fmt.Sprintf("dispatched to %s", config.NodeName)
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("State"), color.CyanString(state))
	}
	fmt.Fprintln(w, "===")
}

func printScrubbed(w io.Writer, data []byte) {
	scrubbed, err := scrubber.ScrubYaml(data)
	if err == nil {
		fmt.Fprintln(w, string(scrubbed))
	} else {
		fmt.Fprintf(w, "error scrubbing secrets from config: %s\n", err)
	}
}

func printContainerExclusionRulesInfo(w io.Writer, config integration.Config) {
	var msg string
	if config.IsCheckConfig() && config.MetricsExcluded {
		msg = "This configuration matched a metrics container-exclusion rule, so it will not be run by the Agent"
	} else if config.IsLogConfig() && config.LogsExcluded {
		msg = "This configuration matched a logs container-exclusion rule, so it will not be run by the Agent"
	}

	if msg != "" {
		fmt.Fprintln(w, color.BlueString(msg))
	}
}
