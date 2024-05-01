// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `otel-agent` binary, including its subcommands.
package command

import (
	"errors"
	"flag"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"go.opentelemetry.io/collector/featuregate"
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
		return &subcommands.GlobalParams{
			ConfPath:   globalParams.ConfPath,
			ConfigName: globalParams.ConfigName,
			LoggerName: loggerName,
		}
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

	otelAgentCmd.PersistentFlags().StringVarP(&globalParams.ConfPath, "config", "c", defaultConfigPath, "path to directory containing datadog.yaml")

	return &otelAgentCmd
}

const configFlag = "config"

func flags(reg *featuregate.Registry, cfgs *subcommands.GlobalParams) *flag.FlagSet {
	flagSet := new(flag.FlagSet)

	flagSet.Var(cfgs, configFlag, "Locations to the config file(s), note that only a"+
		" single location can be set per flag entry e.g. `--config=file:/path/to/first --config=file:path/to/second`.")

	flagSet.Func("set",
		"Set arbitrary component config property. The component has to be defined in the config file and the flag"+
			" has a higher precedence. Array config properties are overridden and maps are joined. Example --set=processors.batch.timeout=2s",
		func(s string) error {
			idx := strings.Index(s, "=")
			if idx == -1 {
				// No need for more context, see TestSetFlag/invalid_set.
				return errors.New("missing equal sign")
			}
			cfgs.Sets = append(cfgs.Sets, "yaml:"+strings.TrimSpace(strings.ReplaceAll(s[:idx], ".", "::"))+": "+strings.TrimSpace(s[idx+1:]))
			return nil
		})

	reg.RegisterFlags(flagSet)
	return flagSet
}
