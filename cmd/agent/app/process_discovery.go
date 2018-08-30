// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/procdiscovery"
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

	var discovered procdiscovery.DiscoveredIntegrations
	err = json.Unmarshal(response, &discovered)
	if err != nil {
		return fmt.Errorf("Couldn't decode agent response: %s", err)
	}

	fmt.Fprintln(color.Output, discovered.Render())
	return nil
}
