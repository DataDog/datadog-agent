// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/info"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

// MakeRootCommand is the root command for the trace-agent
// Please note that the trace-agent can be launched directly
// by the root command, unlike other agents which are managed
// with subcommands.
func MakeRootCommand(defaultLogFile string) *cobra.Command {
	globalParams := subcommands.GlobalParams{
		ConfigName: "datadog",
	}

	commands := makeCommands(&globalParams)
	traceAgentCmd := *commands[0] // first command in the slice is run() command
	// shallow copy should suffice
	traceAgentCmd.Use = "trace-agent [command]"
	traceAgentCmd.Short = "Datadog trace-agent at your service."

	traceAgentCmd.PersistentFlags().StringVarP(&globalParams.ConfPath, "config", "c", "", "path to directory containing datadog.yaml")

	for _, cmd := range commands {
		traceAgentCmd.AddCommand(cmd)
	}

	return &traceAgentCmd
}

func makeCommands(globalParams *subcommands.GlobalParams) []*cobra.Command {
	globalConfGetter := func() subcommands.GlobalParams {
		return subcommands.GlobalParams{
			ConfPath:   globalParams.ConfPath,
			ConfigName: globalParams.ConfigName,
			LoggerName: "TRACE",
		}
	}
	return []*cobra.Command{
		run.MakeCommand(globalConfGetter), // should always be first in the slice
		info.MakeCommand(globalConfGetter),
		version.MakeCommand("trace-agent"),
	}
}
