// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
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

	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// log level is always off since this might be use by other agent to get the hostname
	err = config.SetupLogger(loggerName, "off", "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return fmt.Errorf("Error getting the hostname: %v", err)
	}

	fmt.Println(hname)
	return nil
}
