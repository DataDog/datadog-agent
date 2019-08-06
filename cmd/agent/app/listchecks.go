// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	// TODO: re-enable when the API endpoint is implemented
	// AgentCmd.AddCommand(listCheckCommand)
}

var listCheckCommand = &cobra.Command{
	Use:   "list-checks",
	Short: "Query the agent for the list of checks running",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return doListChecks()
	},
}

// query for the version
func doListChecks() error {
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/check/", config.Datadog.GetInt("cmd_port"))

	body, e := util.DoGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Checks: %s\n", body)
	return nil
}
