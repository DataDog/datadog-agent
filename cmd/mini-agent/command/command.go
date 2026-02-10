// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `mini-agent` binary, including its subcommands.
package command

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/mini-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/mini-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

const (
	// loggerName is the application logger identifier
	loggerName = "MINI-AGENT"
)

// MakeRootCommand is the root command for the mini-agent
func MakeRootCommand() *cobra.Command {
	globalParams := subcommands.GlobalParams{
		ConfigName: "datadog",
		LoggerName: loggerName,
	}

	return makeCommands(&globalParams)
}

func makeCommands(globalParams *subcommands.GlobalParams) *cobra.Command {
	globalConfGetter := func() *subcommands.GlobalParams {
		return globalParams
	}

	commands := []*cobra.Command{
		run.MakeCommand(globalConfGetter),
		version.MakeCommand("mini-agent"),
	}

	// Root cmd is `run()`
	miniAgentCmd := *commands[0]
	miniAgentCmd.Use = "mini-agent [command]"
	miniAgentCmd.Short = "Datadog mini-agent - minimal agent with tagger and metrics"

	for _, cmd := range commands {
		miniAgentCmd.AddCommand(cmd)
	}

	// Add config path flag
	miniAgentCmd.PersistentFlags().StringVarP(&globalParams.ConfPath, "config", "c", "", "Path to datadog.yaml config file")

	return &miniAgentCmd
}
