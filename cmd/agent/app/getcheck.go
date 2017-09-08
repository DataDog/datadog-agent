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
	AgentCmd.AddCommand(getCheckCommand)
	getCheckCommand.Flags().StringVarP(&checkname, "checkname", "c", "", "name of check")
}

var getCheckCommand = &cobra.Command{
	Use:   "getcheck",
	Short: "Query the running agent for the status of a given check.",
	Long:  ``,
	RunE:  doGetCheck,
}

// query for the version
func doGetCheck(cmd *cobra.Command, args []string) error {

	if len(checkname) == 0 {
		return fmt.Errorf("Must supply a check name to query")
	}
	c := GetClient()
	urlstr := "http://" + sockname + "/check/" + checkname
	var e error
	var body []byte
	body, e = common.DoGet(c, urlstr)

	if e != nil {
		fmt.Printf("Error getting check status for check %s: %s\n", checkname, e)
		return e
	}
	fmt.Printf("Check %s status: %s\n", checkname, body)
	return nil
}
