// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `trace-agent` binary, including its subcommands.
package command

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/config"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/controlsvc"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/coverage"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/info"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
)

const (
	// LoggerName is the application logger identifier
	LoggerName = "TRACE"
)

// MakeRootCommand is the root command for the trace-agent
// Please note that the trace-agent can be launched directly
// by the root command, unlike other agents that are managed
// with subcommands.
func MakeRootCommand() *cobra.Command {
	globalParams := subcommands.GlobalParams{
		ConfigName: "datadog-trace",
	}

	return makeCommands(&globalParams)
}

func makeCommands(globalParams *subcommands.GlobalParams) *cobra.Command {
	globalConfGetter := func() *subcommands.GlobalParams {
		return &subcommands.GlobalParams{
			ConfPath:             resolveConfPath(globalParams.ConfPath),
			ConfigName:           globalParams.ConfigName,
			LoggerName:           LoggerName,
			FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
		}
	}
	commands := []*cobra.Command{
		run.MakeCommand(globalConfGetter),
		info.MakeCommand(globalConfGetter),
		version.MakeCommand("trace-agent"),
		config.MakeCommand(globalConfGetter),
	}

	commands = append(commands, controlsvc.Commands(globalConfGetter)...)

	traceAgentCmd := *commands[0] // root cmd is `run()`; indexed at 0
	traceAgentCmd.Use = "trace-agent [command]"
	traceAgentCmd.Short = "Datadog trace-agent at your service."

	for _, cmd := range commands {
		traceAgentCmd.AddCommand(cmd)
	}

	coverage.SetupCoverageCommand(globalConfGetter, &traceAgentCmd)

	traceAgentCmd.PersistentFlags().StringVarP(&globalParams.ConfPath, "config", "c", "",
		"path to datadog.yaml (or a directory containing it); defaults to "+defaultConfigPath+" when that file exists, otherwise the agent runs with environment-variable configuration only")
	traceAgentCmd.PersistentFlags().StringVarP(&globalParams.FleetPoliciesDirPath, "fleetcfgpath", "", "", "path to the directory containing fleet policies")
	_ = traceAgentCmd.PersistentFlags().MarkHidden("fleetcfgpath")

	return &traceAgentCmd
}

// resolveConfPath returns the config path that should be passed to the
// configuration component. When the user did not pass --config explicitly
// (confPath is empty), the platform default path is used only if a file
// actually exists there. Otherwise the empty string is returned, which
// signals comp/core/config to tolerate a missing config file and rely on
// environment variables and built-in defaults.
func resolveConfPath(confPath string) string {
	if confPath != "" {
		return confPath
	}
	if _, err := os.Stat(defaultConfigPath); err == nil {
		return defaultConfigPath
	}
	return ""
}
