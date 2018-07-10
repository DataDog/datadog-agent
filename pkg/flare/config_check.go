// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"fmt"
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	json "github.com/json-iterator/go"
)

// ConfigCheckURL contains the Agent API endpoint URL exposing the loaded checks
var ConfigCheckURL = fmt.Sprintf("https://localhost:%v/agent/config-check", config.Datadog.GetInt("cmd_port"))

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

	r, err := util.DoGet(c, ConfigCheckURL)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintln(w, fmt.Sprintf("The agent ran into an error while checking config: %s", string(r)))
		} else {
			fmt.Fprintln(w, fmt.Sprintf("Failed to query the agent (running?): %s", err))
		}
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
		fmt.Fprintln(w, fmt.Sprintf("\n=== %s check ===", color.GreenString(c.Name)))
		if len(c.Provider) > 0 {
			fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("Source"), color.CyanString(c.Provider)))
		} else {
			fmt.Fprintln(w, fmt.Sprintf("%s: %s", color.BlueString("Source"), color.RedString("Unknown provider")))
		}
		for i, inst := range c.Instances {
			fmt.Fprintln(w, fmt.Sprintf("%s %s:", color.BlueString("Instance"), color.CyanString(strconv.Itoa(i+1))))
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
		fmt.Fprintln(w, "===")
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
			for ids, config := range cr.Unresolved {
				fmt.Fprintln(w, fmt.Sprintf("\n%s: %s", color.BlueString("Auto-discovery IDs"), color.YellowString(ids)))
				fmt.Fprintln(w, fmt.Sprintf("%s:", color.BlueString("Template")))
				fmt.Fprintln(w, config.String())
			}
		}
	}

	return nil
}
