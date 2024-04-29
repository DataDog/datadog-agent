// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package command holds command related files
package command

import (
	"fmt"
	"path"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfigFilePaths      []string
	SysProbeConfFilePath string
}

// SubcommandFactory returns a sub-command factory
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// LoggerName defines the logger name
const LoggerName = "SECURITY"

var (
	defaultSecurityAgentConfigFilePaths = []string{
		path.Join(commonpath.DefaultConfPath, "datadog.yaml"),
		path.Join(commonpath.DefaultConfPath, "security-agent.yaml"),
	}

	defaultSysProbeConfPath = path.Join(commonpath.DefaultConfPath, "system-probe.yaml")
)

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
				return fmt.Errorf("no Security Agent config files to load, exiting")
			}
			return nil
		},
	}

	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&globalParams.ConfigFilePaths, "cfgpath", "c", defaultSecurityAgentConfigFilePaths, "paths to yaml configuration files")
	SecurityAgentCmd.PersistentFlags().StringVar(&globalParams.SysProbeConfFilePath, "sysprobe-config", defaultSysProbeConfPath, "path to system-probe.yaml config")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			SecurityAgentCmd.AddCommand(subcmd)
		}
	}

	return SecurityAgentCmd
}
