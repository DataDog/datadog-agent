// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cmdimport implements 'agent import'.
package cmdimport

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	force := false

	importCmd := &cobra.Command{
		Use:   "import <old_configuration_dir> <destination_dir>",
		Short: "Import and convert configuration files from previous versions of the Agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("please provide all the required arguments")
			}

			if globalParams.ConfFilePath != "" {
				fmt.Fprintf(os.Stderr, "Please note configdir option has no effect\n")
			}
			oldConfigDir := args[0]
			newConfigDir := args[1]

			return common.ImportConfig(oldConfigDir, newConfigDir, force)
		},
		SilenceUsage: true,
	}

	// local flags
	importCmd.Flags().BoolVarP(&force, "force", "f", force, "overwrite existing files")

	return []*cobra.Command{importCmd}
}
