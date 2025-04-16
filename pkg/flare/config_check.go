// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// PrintConfigCheck prints a human-readable representation of the config check response
func PrintConfigCheck(w io.Writer, cr integration.ConfigCheckResponse, withDebug bool) {
	if w != color.Output {
		color.NoColor = true
	}

	if len(cr.ConfigErrors) > 0 {
		fmt.Fprintf(w, "=== Configuration %s ===\n", color.RedString("errors"))
		for check, error := range cr.ConfigErrors {
			fmt.Fprintf(w, "\n%s: %s\n", color.RedString(check), error)
		}
	}

	for _, configResponse := range cr.Configs {
		PrintConfigWithInstanceIDs(w, configResponse.Config, configResponse.InstanceIDs, "")
	}

	if withDebug {
		if len(cr.ResolveWarnings) > 0 {
			fmt.Fprintf(w, "\n=== Resolve %s ===\n", color.YellowString("warnings"))
			for check, warnings := range cr.ResolveWarnings {
				fmt.Fprintf(w, "\n%s\n", color.YellowString(check))
				for _, warning := range warnings {
					fmt.Fprintf(w, "* %s\n", warning)
				}
			}
		}
		if len(cr.Unresolved) > 0 {
			fmt.Fprintf(w, "\n=== %s Configs ===\n", color.YellowString("Unresolved"))
			for ids, configs := range cr.Unresolved {
				fmt.Fprintf(w, "\n%s: %s\n", color.BlueString("Auto-discovery IDs"), color.YellowString(ids))
				fmt.Fprintf(w, "%s:\n", color.BlueString("Templates"))
				for _, config := range configs {
					fmt.Fprintln(w, config.String())
				}
			}
		}
	}
}

// PrintConfigWithInstanceIDs prints a human-readable representation of a configuration with any secrets scrubbed.
// We provide the instanceIDs precomputed from the server as the config information is scrubbed
func PrintConfigWithInstanceIDs(w io.Writer, c integration.Config, instanceIDs []string, checkName string) {
	if checkName != "" && c.Name != checkName {
		return
	}
	if !c.ClusterCheck {
		fmt.Fprintf(w, "\n=== %s check ===\n", color.GreenString(c.Name))
	} else {
		fmt.Fprintf(w, "\n=== %s cluster check ===\n", color.GreenString(c.Name))
	}

	if c.Provider != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.CyanString(c.Provider))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.RedString("Unknown provider"))
	}
	if c.Source != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.CyanString(c.Source))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.RedString("Unknown configuration source"))
	}
	for idx, inst := range c.Instances {
		ID := instanceIDs[idx]
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Config for instance ID"), color.CyanString(ID))
		fmt.Fprintln(w, string(inst))
		fmt.Fprintln(w, "~")
	}
	if len(c.InitConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Init Config"))
		fmt.Fprintln(w, string(c.InitConfig))
	}
	if len(c.MetricConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Metric Config"))
		fmt.Fprintln(w, string(c.MetricConfig))
	}
	if len(c.LogsConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Log Config"))
		fmt.Fprintln(w, string(c.LogsConfig))
	}
	if c.IsTemplate() {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Auto-discovery IDs"))
		for _, id := range c.ADIdentifiers {
			fmt.Fprintf(w, "* %s\n", color.CyanString(id))
		}
		printContainerExclusionRulesInfo(w, &c)
	}
	if c.NodeName != "" {
		state := fmt.Sprintf("dispatched to %s", c.NodeName)
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("State"), color.CyanString(state))
	}
	fmt.Fprintln(w, "===")
}

// PrintClusterCheckConfig prints a human-readable representation of a configuration with any secrets scrubbed.
func PrintClusterCheckConfig(w io.Writer, c integration.Config, checkName string) {
	if checkName != "" && c.Name != checkName {
		return
	}
	configDigest := c.FastDigest()
	if !c.ClusterCheck {
		fmt.Fprintf(w, "\n=== %s check ===\n", color.GreenString(c.Name))
	} else {
		fmt.Fprintf(w, "\n=== %s cluster check ===\n", color.GreenString(c.Name))
	}

	if c.Provider != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.CyanString(c.Provider))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration provider"), color.RedString("Unknown provider"))
	}
	if c.Source != "" {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.CyanString(c.Source))
	} else {
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Configuration source"), color.RedString("Unknown configuration source"))
	}
	for _, inst := range c.Instances {
		ID := string(checkid.BuildID(c.Name, configDigest, inst, c.InitConfig))
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("Config for instance ID"), color.CyanString(ID))
		fmt.Fprintln(w, string(inst))
		fmt.Fprintln(w, "~")
	}
	if len(c.InitConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Init Config"))
		fmt.Fprintln(w, string(c.InitConfig))
	}
	if len(c.MetricConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Metric Config"))
		fmt.Fprintln(w, string(c.MetricConfig))
	}
	if len(c.LogsConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Log Config"))
		fmt.Fprintln(w, string(c.LogsConfig))
	}
	if c.IsTemplate() {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Auto-discovery IDs"))
		for _, id := range c.ADIdentifiers {
			fmt.Fprintf(w, "* %s\n", color.CyanString(id))
		}
		printContainerExclusionRulesInfo(w, &c)
	}
	if c.NodeName != "" {
		state := fmt.Sprintf("dispatched to %s", c.NodeName)
		fmt.Fprintf(w, "%s: %s\n", color.BlueString("State"), color.CyanString(state))
	}
	fmt.Fprintln(w, "===")
}

func printContainerExclusionRulesInfo(w io.Writer, c *integration.Config) {
	var msg string
	if c.IsCheckConfig() && c.MetricsExcluded {
		msg = "This configuration matched a metrics container-exclusion rule, so it will not be run by the Agent"
	} else if c.IsLogConfig() && c.LogsExcluded {
		msg = "This configuration matched a logs container-exclusion rule, so it will not be run by the Agent"
	}

	if msg != "" {
		fmt.Fprintln(w, color.BlueString(msg))
	}
}
