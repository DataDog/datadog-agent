// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

/*
Package app implements the Agent main loop, orchestrating
all the components and providing the command line interface. */
package app

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
)

var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		PersistentPreRunE: preRun,
	}

	// confFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	confFilePath string
	flagNoColor  bool
)

// preRun takes care of common setup, including for now:
//   - parsing of the configuration
//   - handling of the no-color flag
func preRun(_ *cobra.Command, _ []string) error {
	if flagNoColor {
		color.NoColor = true
	}
	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	return nil
}

func init() {
	AgentCmd.PersistentFlags().StringVarP(&confFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	AgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}
