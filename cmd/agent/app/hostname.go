// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

func init() {
	AgentCmd.AddCommand(getHostnameCommand)
}

var getHostnameCommand = &cobra.Command{
	Use:   "hostname",
	Short: "Print the hostname used by the Agent",
	Long:  ``,
	RunE:  doGetHostname,
}

// query for the version
func doGetHostname(cmd *cobra.Command, args []string) error {
	config.SetupLogger("off", "", "", false, true, false)
	err := common.SetupConfigWithoutSecrets(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	hname, err := util.GetHostname()
	if err != nil {
		return fmt.Errorf("Error getting the hostname: %v", err)
	}

	fmt.Println(hname)
	return nil
}
