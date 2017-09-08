// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"

	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/spf13/cobra"
)

func init() {
	// TODO: re-enable when the API endpoint is implemented
	// AgentCmd.AddCommand(reloadCheckCommand)
	checkCmd.SetArgs([]string{"checkName"})
}

var reloadCheckCommand = &cobra.Command{
	Use:   "reload-check <check_name>",
	Short: "Reload a running check",
	Long:  ``,
	RunE:  doreloadCheck,
}

// query for the version
func doreloadCheck(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		checkName = args[0]
	} else {
		return fmt.Errorf("Must supply a check name to query")
	}

	c := common.GetClient()
	urlstr := "http://" + sockname + "/check/" + checkName + "/reload"

	postbody := ""

	body, e := common.DoPost(c, urlstr, "application/json", strings.NewReader(postbody))

	if e != nil {
		return fmt.Errorf("error getting check status for check %s: %v", checkName, e)
	}

	fmt.Printf("Reload check %s: %s\n", checkName, body)
	return nil
}
