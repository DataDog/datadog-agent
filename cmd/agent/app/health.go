// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

var healthVerbose bool

func init() {
	AgentCmd.AddCommand(healthCmd)
	healthCmd.Flags().BoolVarP(&healthVerbose, "verbose", "v", false, "verbose output")
}

var healthCmd = &cobra.Command{
	Use:          "health",
	Short:        "Print the current agent health",
	Long:         ``,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		return requestHealth()
	},
}

func requestHealth() error {
	if flagNoColor {
		color.NoColor = true
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/status/health", config.Datadog.GetInt("cmd_port"))

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	r, err := util.DoGet(c, urlstr)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", err)
		return err
	}

	s := new(health.Status)
	if err = json.Unmarshal(r, s); err != nil {
		return fmt.Errorf("Error unmarshalling json: %s", err)
	}

	if healthVerbose || len(s.Unhealthy) > 0 {
		sort.Strings(s.Unhealthy)
		sort.Strings(s.Healthy)

		if len(s.Unhealthy) > 0 {
			fmt.Fprintln(color.Output, color.RedString("Agent health: FAIL"))
			fmt.Fprintln(color.Output, "=== Unhealthy components ===")
			fmt.Fprintln(color.Output, strings.Join(s.Unhealthy, ", "))
		} else {
			fmt.Fprintln(color.Output, color.GreenString("Agent health: PASS"))
		}
		if len(s.Healthy) > 0 {
			fmt.Fprintln(color.Output, "=== Healthy components ===")
			fmt.Fprintln(color.Output, strings.Join(s.Healthy, ", "))
		}
	}

	if len(s.Unhealthy) > 0 {
		return fmt.Errorf("found %d unhealthy components", len(s.Unhealthy))
	}

	return nil
}
