// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"path"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type GlobalParams struct {
	ConfigFilePaths []string
}

type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

const LoggerName = "SECURITY"

var defaultSecurityAgentConfigFilePaths = []string{
	path.Join(common.DefaultConfPath, "datadog.yaml"),
	path.Join(common.DefaultConfPath, "security-agent.yaml"),
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if flagNoColor {
				color.NoColor = true
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			log.Flush()
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
