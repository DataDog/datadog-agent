// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package command holds command related files
package command

import (
	"path"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath      string
	ExtraConfFilePath []string
}

// SubcommandFactory returns a sub-command factory
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// LoggerName defines the logger name
var (
	defaultConfigFilePaths = []string{
		path.Join(defaultpaths.ConfPath, "datadog.yaml"),
	}
)

// MakeCommand makes the top-level Cobra command for this command.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	var globalParams GlobalParams

	privateActionRunnerCmd := &cobra.Command{
		Use:   "datadog-private-action-runner [command]",
		Short: "Datadog Private Action Runner.",
		Long: `
Datadog Private Action Runner enables execution of private actions.`,
	}

	privateActionRunnerCmd.PersistentFlags().StringArrayVarP(&globalParams.ExtraConfFilePath, "cfgpath", "c", defaultConfigFilePaths, "paths to yaml configuration files")

	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			privateActionRunnerCmd.AddCommand(subcmd)
		}
	}

	return privateActionRunnerCmd
}

// SetDefaultCommandIfNonePresent sets the default command to 'run' if no command is present
func SetDefaultCommandIfNonePresent(cmd *cobra.Command) {
	if len(cmd.Commands()) > 0 {
		cmd.SetArgs([]string{"run"})
	}
}
