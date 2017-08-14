// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"

	"strings"

	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(reloadCheckCommand)
	reloadCheckCommand.Flags().StringVarP(&checkname, "checkname", "c", "", "name of check")
}

var reloadCheckCommand = &cobra.Command{
	Use:   "reloadCheck",
	Short: "Reload a running check.",
	Long:  ``,
	RunE:  doreloadCheck,
}

// query for the version
func doreloadCheck(cmd *cobra.Command, args []string) error {

	if len(checkname) == 0 {
		return fmt.Errorf("Must supply a check name to query")
	}
	c := GetClient()
	urlstr := "http://" + sockname + "/check/" + checkname + "/reload"

	postbody := ""

	body, e := doPost(c, urlstr, "application/json", strings.NewReader(postbody))

	if e != nil {
		fmt.Printf("Error getting check status for check %s: %s\n", checkname, e)
		return e
	}
	fmt.Printf("Reload check %s: %s\n", checkname, body)
	return nil
}
