// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// Package command implements the top-level `otel-agent` binary, including its subcommands.
package command

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/kouhin/envflag"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
	"go.opentelemetry.io/collector/featuregate"
)

const (
	// loggerName is the application logger identifier
	loggerName = "OTELCOL"
)

// MakeRootCommand is the root command for the otel-agent
// Please note that the otel-agent can be launched directly
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

	flagSet := flags(featuregate.GlobalRegistry(), globalParams)
	otelAgentCmd.PersistentFlags().AddGoFlagSet(flagSet)

	// Support these environment variables
	ef := envflag.NewEnvFlag(flagSet, 2,
		map[string]string{ // User-defined env-flag map
			"DD_SYNC_DELAY":  "sync-delay",
			"DD_SYNC_TO":     "sync-to",
			"DD_CORE_CONFIG": "core-config",
		},
		true, // show env variable key in usage
		true, // show env variable value in usage
	)

	if err := ef.Parse(os.Args[1:]); err != nil {
		panic(err)
	}

	return &otelAgentCmd
}

const configFlag = "config"
const coreConfigFlag = "core-config"
const syncDelayFlag = "sync-delay"
const syncTimeoutFlag = "sync-to"

func flags(reg *featuregate.Registry, cfgs *subcommands.GlobalParams) *flag.FlagSet {
	flagSet := new(flag.FlagSet)
	flagSet.Var(cfgs, configFlag, "Locations to the config file(s), note that only a"+
		" single location can be set per flag entry e.g. `--config=file:/path/to/first --config=file:path/to/second`.")
	flagSet.StringVar(&cfgs.CoreConfPath, coreConfigFlag, "", "Location to the Datadog Agent config file.")
	flagSet.DurationVar(&cfgs.SyncDelay, syncDelayFlag, 0, "Delay before first config sync.")
	flagSet.DurationVar(&cfgs.SyncTimeout, syncTimeoutFlag, 3*time.Second, "Timeout for sync requests.")

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
