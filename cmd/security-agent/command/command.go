// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"fmt"
	"path"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/pkg/util/constants"
)

type GlobalParams struct {
	ConfigFilePaths []string
}

type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

const LoggerName = "SECURITY"

var defaultSecurityAgentConfigFilePaths = []string{
	path.Join(constants.DefaultConfPath, "datadog.yaml"),
	path.Join(constants.DefaultConfPath, "security-agent.yaml"),
}

// MakeCommand makes the top-level Cobra command for this command.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	var globalParams GlobalParams
	var flagNoColor bool

	SecurityAgentCmd := &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
		SilenceUsage: true, // don't print usage on errors
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if flagNoColor {
				color.NoColor = true
			}

			if len(globalParams.ConfigFilePaths) == 1 && globalParams.ConfigFilePaths[0] == "" {
				return fmt.Errorf("no Security Agent config files to load, exiting.")
			}
			return nil
		},
	}

	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&globalParams.ConfigFilePaths, flags.CfgPath, "c", defaultSecurityAgentConfigFilePaths, "paths to yaml configuration files")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, flags.NoColor, "n", false, "disable color output")

	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			SecurityAgentCmd.AddCommand(subcmd)
		}
	}

	return SecurityAgentCmd
}
