// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver clusterchecks

package commands

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func GetConfigCheckCobraCmd(flagNoColor *bool, confPath *string, loggerName config.LoggerName) *cobra.Command {
	var withDebug bool
	configCheckCommand := &cobra.Command{
		Use:   "configcheck",
		Short: "Print all configurations loaded & resolved of a running cluster agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {

			if *flagNoColor {
				color.NoColor = true
			}

			// we'll search for a config file named `datadog-cluster.yaml`
			config.Datadog.SetConfigName("datadog-cluster")
			err := common.SetupConfig(*confPath)
			if err != nil {
				return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
			}

			err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			err = flare.GetClusterAgentConfigCheck(color.Output, withDebug)
			if err != nil {
				return err
			}
			return nil
		},
	}
	configCheckCommand.Flags().BoolVarP(&withDebug, "verbose", "v", false, "print additional debug info")
	return configCheckCommand
}
