// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(configCheckCommand)
}

var configCheckCommand = &cobra.Command{
	Use:   "configcheck",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		err = getConfigCheck()
		if err != nil {
			return err
		}
		return nil
	},
}

func getConfigCheck() error {
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
			fmt.Println(fmt.Sprintf("The agent ran into an error while checking config: %s", string(r)))
		} else {
			fmt.Println(fmt.Sprintf("Failed to query the agent (running?): %s", err))
		}
	}

	w := color.Output

	cr := agent.ConfigCheckResponse{}
	err = json.Unmarshal(r, &cr)
	if err != nil {
		fmt.Println("json")
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

	for check, warning := range cr.Warnings {
		fmt.Fprintln(w, check)
		fmt.Fprintln(w, warning)
	}

	return nil
}
