// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command implements the top-level `updater` binary, including its subcommands.
package command

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// common constants for all the updater subcommands.
const (
	ConfigName      = "updater"
	LoggerName      = "UPDATER"
	DefaultLogLevel = "off"
)

// GlobalParams contains the values of updater-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// Package is the package managed by this instance of the updater.
	Package string

	// RepositoriesDir is the path to the directory containing the repositories.
	RepositoriesDir string

	// RunPath is the path to the directory containing the repositories' run data.
	RunPath string
}

// SubcommandFactory is a callable that will return a slice of subcommands.
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this app.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	globalParams := GlobalParams{}

	// AgentCmd is the root command
	agentCmd := &cobra.Command{
		Use:   fmt.Sprintf("%s [command]", os.Args[0]),
		Short: "Datadog Updater at your service.",
		Long: `
Datadog Updater updates your agents based on requests received from the Datadog UI.`,
		SilenceUsage: true,
	}

	agentCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing updater.yaml")
	agentCmd.PersistentFlags().StringVarP(&globalParams.Package, "package", "p", "", "package to update")
	agentCmd.PersistentFlags().StringVarP(&globalParams.RepositoriesDir, "repositories", "d", "/opt/datadog-packages", "path to directory containing repositories")
	agentCmd.PersistentFlags().StringVarP(&globalParams.RunPath, "runpath", "r", "/var/run/datadog-packages", "path to directory containing repositories")
	_ = agentCmd.MarkFlagRequired("package")

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
