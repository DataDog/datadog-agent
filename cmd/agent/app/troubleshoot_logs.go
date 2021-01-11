// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// var (
// 	jsonStatus      bool
// 	prettyPrintJSON bool
// 	statusFilePath  string
// )

func init() {
	AgentCmd.AddCommand(troubleshootLogsCmd)
	// statusCmd.Flags().BoolVarP(&jsonStatus, "json", "j", false, "print out raw json")
	// statusCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	// statusCmd.Flags().StringVarP(&statusFilePath, "file", "o", "", "Output the status command to a file")
	// statusCmd.AddCommand(componentCmd)
	// componentCmd.Flags().BoolVarP(&prettyPrintJSON, "pretty-json", "p", false, "pretty print JSON")
	// componentCmd.Flags().StringVarP(&statusFilePath, "file", "o", "", "Output the status command to a file")
}

var troubleshootLogsCmd = &cobra.Command{
	Use:   "troubleshootLogs",
	Short: "TODO",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfigWithoutSecrets(confFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, config.GetEnv("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		return requestLogTroubleshoot()
	},
}

// var componentCmd = &cobra.Command{
// 	Use:   "component",
// 	Short: "Print the component status",
// 	Long:  ``,
// 	RunE: func(cmd *cobra.Command, args []string) error {
// 		err := common.SetupConfigWithoutSecrets(confFilePath, "")
// 		if err != nil {
// 			return fmt.Errorf("unable to set up global agent configuration: %v", err)
// 		}
// 		if flagNoColor {
// 			color.NoColor = true
// 		}

// 		if len(args) != 1 {
// 			return fmt.Errorf("a component name must be specified")
// 		}
// 		return componentStatus(args[0])
// 	},
// }

func requestLogTroubleshoot() error {

	if !prettyPrintJSON && !jsonStatus {
		fmt.Printf("Getting the status from the agent.\n\n")
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	fmt.Println("https://%v:%v/agent/streamLogs", ipcAddress, config.Datadog.GetInt("cmd_port"))
	urlstr := fmt.Sprintf("https://%v:%v/agent/streamLogs", ipcAddress, config.Datadog.GetInt("cmd_port"))

	err = streamRequest(urlstr, func(chunk []byte) {
		fmt.Print(string(chunk))
	})

	if err != nil {
		return err
	}

	return nil
}
