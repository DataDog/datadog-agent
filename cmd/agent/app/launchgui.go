// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/spf13/cobra"
)

var (
	launchCmd = &cobra.Command{
		Use:          "launchgui",
		Short:        "starts the Datadog Agent GUI",
		Long:         ``,
		RunE:         launchGui,
		SilenceUsage: true,
	}

)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(launchCmd)

}

func launchGui(cmd *cobra.Command, args []string) error {
	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	return gui.LaunchGui()
}
