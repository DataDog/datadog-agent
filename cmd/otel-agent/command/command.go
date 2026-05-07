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
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kouhin/envflag"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/controlsvc"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/coverage"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/flare"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/run"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands/status"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/version"
	"go.opentelemetry.io/collector/featuregate"
)

const (
	// loggerName is the application logger identifier
	loggerName = "OTELCOL"
)

var (
	// BYOC indicates whether the otel agent was built via byoc
	BYOC string
)

// MakeRootCommand is the root command for the otel-agent
// Please note that the otel-agent can be launched directly
// by the root command, unlike other agents that are managed
// with subcommands.
func MakeRootCommand() *cobra.Command {
	globalParams := subcommands.GlobalParams{
		ConfigName: "datadog-otel",
		LoggerName: loggerName,
		BYOC:       strings.EqualFold(BYOC, "true"),
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
		status.MakeCommand(globalConfGetter),
		flare.MakeCommand(globalConfGetter),
	}

	// Add Windows service control commands (noop on non-Windows via stub)
	commands = append(commands, controlsvc.Commands(globalParams)...)

	otelAgentCmd := *commands[0] // root cmd is `run()`; indexed at 0
	otelAgentCmd.Use = "otel-agent [command]"
	otelAgentCmd.Short = "Datadog otel-agent at your service."

	for _, cmd := range commands {
		otelAgentCmd.AddCommand(cmd)
	}

	coverage.SetupCoverageCommand(globalConfGetter, &otelAgentCmd)

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

	if err := validateDurationEnvVars(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// There may be other env vars in addition to the ones in envflag.NewEnvFlag. Do not panic if those env vars do not have a help message (flag.ErrHelp)
	if err := ef.Parse(os.Args[1:]); err != nil && err != flag.ErrHelp {
		log.Fatalf("Error: failed to parse flags: %v", err)
	}

	return &otelAgentCmd
}

const configFlag = "config"
const coreConfigFlag = "core-config"
const syncDelayFlag = "sync-delay" // TODO: Change this to sync-on-init-timeout
const syncTimeoutFlag = "sync-to"

// durationEnvVars maps environment variable names to their flag names for
// duration-type flags. Used by validateDurationEnvVars to produce clear
// diagnostics when a value like "30" is passed instead of "30s".
var durationEnvVars = map[string]string{
	"DD_SYNC_DELAY": syncDelayFlag,
	"DD_SYNC_TO":    syncTimeoutFlag,
}

// validateDurationEnvVars checks that duration-type environment variables
// contain valid Go duration strings. The envflag library loses the original
// time.ParseDuration error, so pre-validating here lets us report the root
// cause (e.g. missing unit suffix) directly to the user.
func validateDurationEnvVars() error {
	for envVar, flagName := range durationEnvVars {
		val, ok := os.LookupEnv(envVar)
		if !ok || val == "" {
			continue
		}
		if _, err := time.ParseDuration(val); err != nil {
			hint := `expected a Go duration string (e.g. "30s", "1m", "500ms")`
			if _, numErr := strconv.ParseFloat(val, 64); numErr == nil {
				hint += fmt.Sprintf("; did you mean %q?", val+"s")
			}
			return fmt.Errorf("invalid value %q for %s (--%s): %w; %s",
				val, envVar, flagName, err, hint)
		}
	}
	return nil
}

func flags(reg *featuregate.Registry, cfgs *subcommands.GlobalParams) *flag.FlagSet {
	flagSet := new(flag.FlagSet)
	flagSet.Var(cfgs, configFlag, "Locations to the config file(s), note that only a"+
		" single location can be set per flag entry e.g. `--config=file:/path/to/first --config=file:path/to/second`.")
	flagSet.StringVar(&cfgs.CoreConfPath, coreConfigFlag, "", "Location to the Datadog Agent config file.")
	flagSet.DurationVar(&cfgs.SyncOnInitTimeout, syncDelayFlag, 0, "How long should config sync retry at initialization before failing.")
	flagSet.DurationVar(&cfgs.SyncTimeout, syncTimeoutFlag, 3*time.Second, "Timeout for config sync requests.")

	flagSet.Func("set",
		"Set arbitrary component config property. The component has to be defined in the config file and the flag"+
			" has a higher precedence. Array config properties are overridden and maps are joined. Example --set=exporters.debug.verbosity=detailed",
		func(s string) error {
			before, after, ok := strings.Cut(s, "=")
			if !ok {
				// No need for more context, see TestSetFlag/invalid_set.
				return errors.New("missing equal sign")
			}
			cfgs.Sets = append(cfgs.Sets, "yaml:"+strings.TrimSpace(strings.ReplaceAll(before, ".", "::"))+": "+strings.TrimSpace(after))
			return nil
		})

	err := featuregate.GlobalRegistry().Set("datadog.EnableOperationAndResourceNameV2", true)
	if err != nil {
		panic(err)
	}
	reg.RegisterFlags(flagSet)
	return flagSet
}
