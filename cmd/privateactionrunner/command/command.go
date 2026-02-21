// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package command holds command related files
package command

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// LoggerName defines the logger name for the private action runner
const LoggerName = "PRIV-ACTION"

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	// ConfFilePath holds the path to the folder containing the configuration
	// file, to allow overrides from the command line
	ConfFilePath string

	// ExtraConfFilePath represents the paths to additional configuration files.
	ExtraConfFilePath []string

	// NoColor is a flag to disable color output
	NoColor bool
}

// SubcommandFactory returns a sub-command factory
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// MakeCommand makes the top-level Cobra command for this command.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	var globalParams GlobalParams

	privateActionRunnerCmd := &cobra.Command{
		Use:   "datadog-agent-action [command]",
		Short: "Datadog Private Action Runner.",
		Long: `
Datadog Private Action Runner enables execution of private actions.`,
		SilenceUsage: true,
	}

	privateActionRunnerCmd.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	privateActionRunnerCmd.PersistentFlags().StringArrayVarP(&globalParams.ExtraConfFilePath, "extracfgpath", "E", []string{}, "specify additional configuration files to be loaded sequentially after the main datadog.yaml")
	privateActionRunnerCmd.PersistentFlags().BoolVarP(&globalParams.NoColor, "no-color", "n", false, "disable color output")

	privateActionRunnerCmd.PersistentPreRun = func(*cobra.Command, []string) {
		if globalParams.NoColor {
			color.NoColor = true
		}
	}
	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			privateActionRunnerCmd.AddCommand(subcmd)
		}
	}

	return privateActionRunnerCmd
}
