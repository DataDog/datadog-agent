// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/controlsvc"
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
		ConfigName: "datadog-trace",
	}

	return makeCommands(&globalParams)
}

func makeCommands(globalParams *subcommands.GlobalParams) *cobra.Command {
	globalConfGetter := func() *subcommands.GlobalParams {
		return &subcommands.GlobalParams{
			ConfPath:   globalParams.ConfPath,
			ConfigName: globalParams.ConfigName,
			LoggerName: "TRACE",
		}
	}
	commands := []*cobra.Command{
		run.MakeCommand(globalConfGetter),
		info.MakeCommand(globalConfGetter),
		version.MakeCommand("trace-agent"),
	}

	commands = append(commands, controlsvc.Commands(globalConfGetter)...)

	traceAgentCmd := *commands[0] // root cmd is `run()`; indexed at 0
	traceAgentCmd.Use = "trace-agent [command]"
	traceAgentCmd.Short = "Datadog trace-agent at your service."

	for _, cmd := range commands {
		traceAgentCmd.AddCommand(cmd)
	}

	traceAgentCmd.PersistentFlags().StringVarP(&globalParams.ConfPath, "config", "c", defaultConfigPath, "path to directory containing datadog.yaml")

	return &traceAgentCmd
}
