// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/fatih/color"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
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

	if configCheckURL == "" {
		configCheckURL = fmt.Sprintf("https://localhost:%v/agent/config-check", config.Datadog.GetInt("cmd_port"))
	}
	r, err := util.DoGet(c, configCheckURL)
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
		fmt.Fprintln(w, fmt.Sprintf("=== Configuration %s ===", color.RedString("errors")))
		for check, error := range cr.ConfigErrors {
			fmt.Fprintln(w, fmt.Sprintf("\n%s: %s", color.RedString(check), error))
		}
	}

	for _, c := range cr.Configs {
		PrintConfig(w, c)
	}

	if withDebug {
		if len(cr.ResolveWarnings) > 0 {
			fmt.Fprintln(w, fmt.Sprintf("\n=== Resolve %s ===", color.YellowString("warnings")))
			for check, warnings := range cr.ResolveWarnings {
				fmt.Fprintln(w, fmt.Sprintf("\n%s", color.YellowString(check)))
				for _, warning := range warnings {
					fmt.Fprintln(w, fmt.Sprintf("* %s", warning))
				}
			}
		}
		if len(cr.Unresolved) > 0 {
			fmt.Fprintln(w, fmt.Sprintf("\n=== %s Configs ===", color.YellowString("Unresolved")))
			for ids, configs := range cr.Unresolved {
				fmt.Fprintln(w, fmt.Sprintf("\n%s: %s", color.BlueString("Auto-discovery IDs"), color.YellowString(ids)))
				fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Templates")))
				for _, config := range configs {
					fmt.Fprintln(w, config.String())
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

// PrintConfig prints a human-readable representation of a configuration
func PrintConfig(w io.Writer, c integration.Config) {
	if !c.ClusterCheck {
		fmt.Fprintln(w, fmt.Sprintf("\n=== %s check ===", color.GreenString(c.Name)))
	} else {
		fmt.Fprintln(w, fmt.Sprintf("\n=== %s cluster check ===", color.GreenString(c.Name)))
	}

	if len(c.Provider) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("Source"), color.CyanString(c.Provider)))
	} else {
		fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("Source"), color.RedString("Unknown provider")))
	}
	for _, inst := range c.Instances {
		ID := string(check.BuildID(c.Name, inst, c.InitConfig))
		fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("Instance ID"), color.CyanString(ID)))
		fmt.Fprint(w, fmt.Sprintf("%s", inst))
		fmt.Fprintln(w, "~")
	}
	if len(c.InitConfig) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Init Config")))
		fmt.Fprintln(w, string(c.InitConfig))
	}
	if len(c.MetricConfig) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Metric Config")))
		fmt.Fprintln(w, string(c.MetricConfig))
	}
	if len(c.LogsConfig) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Log Config")))
		fmt.Fprintln(w, string(c.LogsConfig))
	}
	if len(c.ADIdentifiers) > 0 {
		fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Auto-discovery IDs")))
		for _, id := range c.ADIdentifiers {
			fmt.Fprintln(w, fmt.Sprintf("* %s", color.CyanString(id)))
		}
	}
	if c.NodeName != "" {
		state := fmt.Sprintf("dispatched to %s", c.NodeName)
		fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("State"), color.CyanString(state)))
	}
	fmt.Fprintln(w, "===")
}
