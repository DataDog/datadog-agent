// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(processDiscoveryCmd)
}

var processDiscoveryCmd = &cobra.Command{
	Use:          "discovery",
	Short:        "Print the integrations that could be used on this host",
	Long:         ``,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		return requestDiscoveredIntegrations()
	},
}

func requestDiscoveredIntegrations() error {
	fmt.Printf("Getting the discovered integrations from the agent.\n\n")
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/agent/discovery", config.Datadog.GetInt("cmd_port"))

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	response, err := util.DoGet(c, urlstr)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(response, errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", err)
		return err
	}

	var discovered autodiscovery.DiscoveredIntegrations
	err = json.Unmarshal(response, &discovered)
	if err != nil {
		return fmt.Errorf("Couldn't decode agent response: %s", err)
	}

	running, failing, err := requestIntegrations(c)
	if err != nil {
		return err
	}

	if len(running) != 0 {
		fmt.Println("Running checks:")
		for integration := range running {
			fmt.Fprintln(color.Output, fmt.Sprintf("\t- %s", color.YellowString(integration)))
		}
		fmt.Println("")
	}

	if len(failing) != 0 {
		fmt.Println("Failing checks:")
		for integration := range failing {
			fmt.Fprintln(color.Output, fmt.Sprintf("\t- %s", color.RedString(integration)))
		}
		fmt.Println("")
	}

	if len(discovered) == 0 {
		fmt.Println("There was no new integration found.")
		return nil
	}

	for integration, processes := range discovered {
		_, isRunning := running[integration]
		_, isFailing := failing[integration]

		// Do not show integrations that are already configured
		if !(isRunning || isFailing) {
			header := "Discovered '%s' for processes:"
			if len(processes) == 1 {
				header = "Discovered '%s' for process:"
			}

			fmt.Fprintln(color.Output, fmt.Sprintf(header, color.GreenString(integration)))
			for _, proc := range processes {
				fmt.Fprintln(color.Output, fmt.Sprintf("\t- %s", prettifyCmd(proc.Cmd)))
			}
			fmt.Println("")
		}
	}

	return nil
}

func requestIntegrations(c *http.Client) (map[string]struct{}, map[string]struct{}, error) {
	running := map[string]struct{}{}
	failing := map[string]struct{}{}

	statusUrlstr := fmt.Sprintf("https://localhost:%v/agent/status", config.Datadog.GetInt("cmd_port"))
	statusResponse, err := util.DoGet(c, statusUrlstr)

	if err != nil {
		return running, failing, fmt.Errorf("Couldn't request agent status: %s", err)
	}

	var status struct {
		RunnerStats struct {
			Checks map[string]interface{}
		} `json:"runnerStats"`
		CheckSchedulerStats struct {
			LoaderErrors map[string]interface{}
		} `json:"checkSchedulerStats"`
	}

	err = json.Unmarshal(statusResponse, &status)
	if err != nil {
		return running, failing, fmt.Errorf("Couldn't decode agent status response: %s", err)
	}

	for key := range status.RunnerStats.Checks {
		running[key] = struct{}{}
	}

	for key := range status.CheckSchedulerStats.LoaderErrors {
		failing[key] = struct{}{}
	}

	return running, failing, nil
}

func prettifyCmd(cmd string) string {
	fields := strings.Fields(cmd)

	if len(fields) == 0 {
		return ""
	}

	fields[0] = color.BlueString(fields[0])
	return strings.Join(fields, " ")
}
