// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cmdimport implements 'agent import'.
package cmdimport

import (
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args contains the positional command-line arguments
	args []string

	// force is the value of --force
	force bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	importCmd := &cobra.Command{
		Use:          "import <old_configuration_dir> <destination_dir>",
		Short:        "Import and convert configuration files from previous versions of the Agent",
		Long:         ``,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return fxutil.OneShot(importCmd,
				fx.Supply(cliParams),
			)
		},
	}

	// local flags
	importCmd.Flags().BoolVarP(&cliParams.force, "force", "f", cliParams.force, "overwrite existing files")

	return []*cobra.Command{importCmd}
}

func importCmd(cliParams *cliParams) error {
	if len(cliParams.args) != 2 {
		return fmt.Errorf("please provide all the required arguments")
	}

	if cliParams.ConfFilePath != "" {
		fmt.Fprintf(os.Stderr, "Please note configdir option has no effect\n")
	}
	oldConfigDir := cliParams.args[0]
	newConfigDir := cliParams.args[1]

	return common.ImportConfig(oldConfigDir, newConfigDir, cliParams.force)
}
