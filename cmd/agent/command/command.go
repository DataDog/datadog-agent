// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `agent` binary, including its subcommands.
package command

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	ConfigName = "datadog"
	LoggerName = "CORE"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// SysProbeConfFilePath holds the path to the folder containing the system-probe
	// configuration file, to allow overrides from the command line
	SysProbeConfFilePath string
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// GetDefaultCoreBundleParams returns the default params for the Core Bundle (config loaded from the "datadog" file,
// without secrets and logger disabled).
func GetDefaultCoreBundleParams(globalParams *GlobalParams) core.BundleParams {
	return core.BundleParams{
		ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath),
		LogParams:    log.LogForOneShot(LoggerName, "off", true)}
}

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		// cobra will tokenize the "Use" string by space, and take the first one so there's no need to pass anything
		// besides the filename of the executable.
		// Not even '[command]' is respected - try using their example "add [-F file | -D dir]... [-f format] profile"
		// and it will still come out as "add [command]" in the help output.
		// If the file name contains a space, this will break - but this is not the case for the Agent executable.
		Use:   filepath.Base(os.Args[0]),
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them
to Datadog on your behalf so that you can do something useful with your
monitoring and performance data.`,
		SilenceUsage: true,
	}

	agentCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	agentCmd.PersistentFlags().StringVarP(&globalParams.SysProbeConfFilePath, "sysprobecfgpath", "", "", "path to directory containing system-probe.yaml")

	// github.com/fatih/color sets its global color.NoColor to a default value based on
	// whether the process is running in a tty.  So, we only want to override that when
	// the value is true.
	var noColorFlag bool
	agentCmd.PersistentFlags().BoolVarP(&noColorFlag, "no-color", "n", false, "disable color output")
	agentCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if noColorFlag {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			agentCmd.AddCommand(cmd)
		}
	}

	return agentCmd
}
