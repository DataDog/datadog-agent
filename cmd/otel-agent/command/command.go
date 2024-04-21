// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `otel-agent` binary, including its subcommands.
package command

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	// loggerName is the application logger identifier
	loggerName = "OTELCOL"
)

// defaultConfigPath specifies the default configuration file path for non-Windows systems.
var defaultConfigPath = filepath.Join(setup.InstallPath, "etc/otel-config.yaml")

// MakeRootCommand is the root command for the trace-agent
// Please note that the trace-agent can be launched directly
// by the root command, unlike other agents that are managed
// with subcommands.
func MakeRootCommand() *cobra.Command {
	globalParams := subcommands.GlobalParams{
		ConfigName: "datadog-otel",
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
		version.MakeCommand("otel-agent"),
	}

	otelAgentCmd := *commands[0] // root cmd is `run()`; indexed at 0
	otelAgentCmd.Use = "otel-agent [command]"
	otelAgentCmd.Short = "Datadog otel-agent at your service."

	for _, cmd := range commands {
		otelAgentCmd.AddCommand(cmd)
	}

	otelAgentCmd.PersistentFlags().StringSliceVarP(&globalParams.ConfPaths, "config", "c", []string{defaultConfigPath}, "path to the configuration file")

	return &otelAgentCmd
}
