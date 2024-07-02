// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	runCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/cmd/run/common"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

func disableDefenderCmd(cmd *cobra.Command, _ []string) error {
	host, err := runCommon.CreateRemoteHost(cmd)
	if err != nil {
		return err
	}

	fmt.Printf("Disabling Windows Defender on %s\n", host.Address)
	err = windowsCommon.DisableDefender(host)
	if err != nil {
		return err
	}
	return nil
}

// Init adds commands to the root command
func Init(rootCmd *cobra.Command) {
	var disableDefenderCmd = &cobra.Command{
		Use:   "disable-defender",
		Short: "Disable Windows Defender",
		Long:  "Disable Windows Defender",
		RunE:  disableDefenderCmd,
	}

	rootCmd.AddCommand(disableDefenderCmd)
}
