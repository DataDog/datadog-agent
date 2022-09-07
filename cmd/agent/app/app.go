// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package app implements the top-level `agent` binary, including its subcommands.
package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// SubcommandFactory is a callable that will return a subcommand.
type SubcommandFactory func(globalArgs *GlobalArgs) *cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalArgs := GlobalArgs{
		// LoggerName is the name of the core agent logger
		LoggerName: "CORE",
	}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		SilenceUsage: true,
	}

	agentCmd.PersistentFlags().StringVarP(&globalArgs.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	agentCmd.PersistentFlags().BoolVarP(&globalArgs.FlagNoColor, "no-color", "n", false, "disable color output")
	agentCmd.PersistentFlags().StringVarP(&globalArgs.SysProbeConfFilePath, "sysprobecfgpath", "", "", "path to directory containing system-probe.yaml")

	for _, sf := range subcommandFactories {
		agentCmd.AddCommand(sf(&globalArgs))
	}
	return agentCmd
}
