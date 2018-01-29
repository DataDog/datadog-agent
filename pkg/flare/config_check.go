// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package flare

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
)

// GetConfigCheck dump all loaded configurations to the writer
func GetConfigCheck(w io.Writer, withResolveWarnings bool) error {
	if w != color.Output {
		color.NoColor = true
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/config-check", config.Datadog.GetInt("cmd_port"))

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	r, err := util.DoGet(c, urlstr)
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

	for provider, configs := range cr.Configs {
		fmt.Fprintln(w, fmt.Sprintf("====== Provider %s ======", color.BlueString(provider)))
		for _, c := range configs {
			fmt.Fprintln(w, fmt.Sprintf("\n--- Check %s ---", color.GreenString(c.Name)))
			for i, inst := range c.Instances {
				fmt.Fprintln(w, fmt.Sprintf("*** Instance %s", color.CyanString(strconv.Itoa(i+1))))
				fmt.Fprint(w, fmt.Sprintf("%s", inst))
				fmt.Fprintln(w, strings.Repeat("*", 3))
			}
			fmt.Fprintln(w, fmt.Sprintf("Init Config: %s", c.InitConfig))
			fmt.Fprintln(w, fmt.Sprintf("Metric Config: %s", c.MetricConfig))
			fmt.Fprintln(w, fmt.Sprintf("Log Config: %s", c.LogsConfig))
			fmt.Fprintln(w, strings.Repeat("-", 3))
		}
		fmt.Fprintln(w, strings.Repeat("=", 10))
	}

	if withResolveWarnings {
		for check, warnings := range cr.Warnings {
			fmt.Fprintln(w, check)
			for _, warning := range warnings {
				fmt.Fprintln(w, warning)
			}
		}
	}

	return nil
}
