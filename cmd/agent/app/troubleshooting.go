// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	metadataEndpoint = "/agent/metadata/"
)

var (
	troubleshootingCmd = &cobra.Command{
		Use:   "troubleshooting",
		Short: "Helpers to troubleshoot the Datadog Agent.",
		Long: `
This command offers a list of helpers to troubleshoot the Datadog Agent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help() //nolint:errcheck
			os.Exit(0)
			return nil
		},
	}

	payloadV5Cmd = &cobra.Command{
		Use:   "metadata_v5",
		Short: "Print the metadata payload for the agent.",
		Long: `
This command print the V5 metadata payload for the Agent. This payload is used to populate the infra list and host map in Datadog. It's called 'V5' because it's the same payload sent since Agent V5. This payload is mandatory in order to create a new host in Datadog.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPayload("v5")
		},
	}

	payloadInventoriesCmd = &cobra.Command{
		Use:   "metadata_inventory",
		Short: "Print the Inventory metadata payload for the agent.",
		Long: `
This command print the last Inventory metadata payload sent by the Agent. This payload is used by the 'inventories/sql' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printPayload("inventory")
		},
	}
)

func init() {
	troubleshootingCmd.AddCommand(payloadV5Cmd)
	troubleshootingCmd.AddCommand(payloadInventoriesCmd)
	AgentCmd.AddCommand(troubleshootingCmd)
}

func printPayload(payloadName string) error {
	if flagNoColor {
		color.NoColor = true
	}

	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		fmt.Printf("unable to set up global agent configuration: %v\n", err)
		return nil
	}

	err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	if err := util.SetAuthToken(); err != nil {
		fmt.Println(err)
		return nil
	}

	c := util.GetClient(false)
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%d%s%s", ipcAddress, config.Datadog.GetInt("cmd_port"), metadataEndpoint, payloadName)

	r, err := util.DoGet(c, apiConfigURL, util.CloseConnection)
	if err != nil {
		return fmt.Errorf("Could not fetch metadata v5 payload: %s", err)
	}

	fmt.Println(string(r))
	return nil
}
