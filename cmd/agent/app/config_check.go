// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/flare"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var withResolveErrors bool

func init() {
	AgentCmd.AddCommand(configCheckCommand)

	configCheckCommand.Flags().BoolVarP(&withResolveErrors, "verbose", "v", false, "prints resolve warnings/errors")
}

var configCheckCommand = &cobra.Command{
	Use:   "configcheck",
	Short: "Execute some connectivity diagnosis on your system",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		err = flare.GetConfigCheck(color.Output, withResolveErrors)
		if err != nil {
			return err
		}
		return nil
	},
}
