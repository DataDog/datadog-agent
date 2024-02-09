// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command contains utilities for creating system-probe commands
package command

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// GlobalParams contains the values of system-probe global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	// AgentCmd is the root command
	sysprobeCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Agent System Probe",
		Long: `
The Datadog Agent System Probe runs as superuser in order to instrument
your machine at a deeper level. It is required for features such as Network Performance Monitoring,
Runtime Security Monitoring, Universal Service Monitoring, and others.`,
		SilenceUsage: true,
	}

	sysprobeCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "config", "c", "", "path to directory containing system-probe.yaml")

	// github.com/fatih/color sets its global color.NoColor to a default value based on
	// whether the process is running in a tty.  So, we only want to override that when
	// the value is true.
	var noColorFlag bool
	sysprobeCmd.PersistentFlags().BoolVarP(&noColorFlag, "no-color", "n", false, "disable color output")
	sysprobeCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if noColorFlag {
			color.NoColor = true
		}
	}

	for _, sf := range subcommandFactories {
		subcommands := sf(&globalParams)
		for _, cmd := range subcommands {
			sysprobeCmd.AddCommand(cmd)
		}
	}

	return sysprobeCmd
}

// SetDefaultCommandIfNonePresent sets the "run" subcommand as root command
// to support the legacy (without any command) invocation.
func SetDefaultCommandIfNonePresent(rootCmd *cobra.Command) {
	var subCommandNames []string
	for _, command := range rootCmd.Commands() {
		subCommandNames = append(subCommandNames, append(command.Aliases, command.Name())...)
	}

	args := []string{os.Args[0], "run"}
	if len(os.Args) > 1 {
		potentialCommand := os.Args[1]
		if potentialCommand == "help" || potentialCommand == "completion" {
			return
		}

		for _, command := range subCommandNames {
			if command == potentialCommand {
				return
			}
		}
		if !strings.HasPrefix(potentialCommand, "-") {
			// run command takes no positional arguments, so if one is passed
			// fallback to default cobra handling for good errors
			return
		}
		args = append(args, os.Args[1:]...)
	}
	os.Args = args
}
