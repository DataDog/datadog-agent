// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/pkg/flare"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var withDebug bool

func init() {
	AgentCmd.AddCommand(configCheckCommand)

	configCheckCommand.Flags().BoolVarP(&withDebug, "verbose", "v", false, "print additional debug info")
}

var configCheckCommand = &cobra.Command{
	Use:   "configcheck",
	Short: "Print all configurations loaded & resolved of a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := flare.GetConfigCheck(color.Output, withDebug)
		if err != nil {
			return err
		}
		return nil
	},
}
