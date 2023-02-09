// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/start"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

// MakeRootCommand is the root command for the trace-agent
// Please note that the trace-agent can be launched directly
// by the root command, unlike other agents which are managed
// with subcommands.
func MakeRootCommand(defaultLogFile string) *cobra.Command {

	cliParams := &start.CLIParams{}

	// traceAgentCmd is the root command
	traceAgentCmd := &cobra.Command{
		Use:   "trace-agent [command]",
		Short: "Datadog trace-agent at your service.",
		Long: `
The Datadog trace-agent aggregates, samples, and forwards traces to datadog
submitted by tracers loaded into your application.`,
		RunE: func(*cobra.Command, []string) error {
			return start.RunTraceAgentFct(cliParams, "", defaultLogFile, start.Start)
		},
	}

	traceAgentCmd.PersistentFlags().StringVarP(&cliParams.ConfPath, "config", "c", "", "path to directory containing datadog.yaml")
	traceAgentCmd.PersistentFlags().StringVarP(&cliParams.PIDPath, "pid", "p", "", "path for the PID file to be created")

	for _, cmd := range makeCommands(defaultLogFile) {
		traceAgentCmd.AddCommand(cmd)
	}

	return traceAgentCmd

}

func makeCommands(defaultLogFile string) []*cobra.Command {
	return []*cobra.Command{start.MakeCommand(defaultLogFile), version.MakeCommand("trace-agent")}
}
