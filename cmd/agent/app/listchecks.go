// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
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
	RunE:  doListChecks,
}

// query for the version
func doListChecks(cmd *cobra.Command, args []string) error {
	c := common.GetClient()
	urlstr := "http://" + sockname + "/check/"

	body, e := common.DoGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Checks: %s\n", body)
	return nil
}
