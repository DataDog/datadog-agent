// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/common"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	importCmd = &cobra.Command{
		Use:          "import <old_configuration_dir> <destination_dir>",
		Short:        "Import and convert configuration files from previous versions of the Agent",
		Long:         ``,
		RunE:         doImport,
		SilenceUsage: true,
	}

	force = false
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(importCmd)

	// local flags
	importCmd.Flags().BoolVarP(&force, "force", "f", force, "overwrite existing files")
}

func doImport(cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("please provide all the required arguments")
	}

	if confFilePath != "" {
		fmt.Fprintf(os.Stderr, "Please note configdir option has no effect\n")
	}
	oldConfigDir := args[0]
	newConfigDir := args[1]

	if flagNoColor {
		color.NoColor = true
	}

	return common.ImportConfig(oldConfigDir, newConfigDir, force)
}
