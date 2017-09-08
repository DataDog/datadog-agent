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
	AgentCmd.AddCommand(getVersionCommand)

}

var getVersionCommand = &cobra.Command{
	Use:   "getversion",
	Short: "Query the running agent for the software version.",
	Long:  ``,
	RunE:  doGetVersion,
}

// query for the version
func doGetVersion(cmd *cobra.Command, args []string) error {
	c := GetClient()
	urlstr := "http://" + sockname + "/agent/version"

	body, e := common.DoGet(c, urlstr)
	if e != nil {
		fmt.Printf("Error getting version string: %s\n", e)
		return e
	}
	fmt.Printf("Version: %s\n", body)
	return nil
}
