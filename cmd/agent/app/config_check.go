// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}
		var b bytes.Buffer
		color.Output = &b
		err = flare.GetConfigCheck(color.Output, withDebug)
		if err != nil {
			return fmt.Errorf("unable to get config: %v", err)
		}

		scrubbed, err := log.CredentialsCleanerBytes(b.Bytes())
		if err != nil {
			return fmt.Errorf("unable to scrub sensitive data configcheck output: %v", err)
		}

		fmt.Println(string(scrubbed))
		return nil
	},
}
