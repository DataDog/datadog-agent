// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// configCheckURL contains the Agent API endpoint URL exposing the loaded checks
var configCheckURL string

// GetConfigCheck dump all loaded configurations to the writer
func GetConfigCheck(w io.Writer, withDebug bool) error {
	if w != color.Output {
		color.NoColor = true
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	if configCheckURL == "" {
		configCheckURL = fmt.Sprintf("https://%v:%v/agent/config-check", ipcAddress, config.Datadog.GetInt("cmd_port"))
	}
	r, err := util.DoGet(c, configCheckURL, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while checking config: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	cr := response.ConfigCheckResponse{}
	err = json.Unmarshal(r, &cr)
	if err != nil {
		return err
	}

	if len(cr.ConfigErrors) > 0 {
		fmt.Fprintf(w, "=== Configuration %s ===\n", color.RedString("errors"))
		for check, error := range cr.ConfigErrors {
			fmt.Fprintf(w, "\n%s: %s\n", color.RedString(check), error)
		}
	}

	for _, c := range cr.Configs {
		PrintConfig(w, c, "")
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
					printYaml(w, []byte(config.String()))
				}
			}
		}
	}

	return nil
}

// GetClusterAgentConfigCheck proxies GetConfigCheck overidding the URL
func GetClusterAgentConfigCheck(w io.Writer, withDebug bool) error {
	configCheckURL = fmt.Sprintf("https://localhost:%v/config-check", config.Datadog.GetInt("cluster_agent.cmd_port"))
	return GetConfigCheck(w, withDebug)
}

func printYaml(w io.Writer, data []byte) {
	scrubbed, err := scrubber.ScrubYaml(data)
	if err == nil {
		fmt.Fprintln(w, string(scrubbed))
	} else {
		fmt.Fprintf(w, "error scrubbing secrets from config: %s\n", err)
	}
}

// PrintConfig prints a human-readable representation of a configuration
func PrintConfig(w io.Writer, c integration.Config, checkName string) {
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
		printYaml(w, inst)
		fmt.Fprintln(w, "~")
	}
	if len(c.InitConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Init Config"))
		printYaml(w, c.InitConfig)
	}
	if len(c.MetricConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Metric Config"))
		printYaml(w, c.MetricConfig)
	}
	if len(c.LogsConfig) > 0 {
		fmt.Fprintf(w, "%s:\n", color.BlueString("Log Config"))
		printYaml(w, c.LogsConfig)
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
