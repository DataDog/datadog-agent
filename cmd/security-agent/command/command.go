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

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfigFilePaths      []string
	SysProbeConfFilePath string
	FleetPoliciesDirPath string

	// NoColor is a flag to disable color output
	NoColor bool
}

// SubcommandFactory returns a sub-command factory
type SubcommandFactory func(globalParams *GlobalParams) []*cobra.Command

// LoggerName defines the logger name
const LoggerName = "SECURITY"

var (
	defaultSecurityAgentConfigFilePaths = []string{
		path.Join(defaultpaths.ConfPath, "datadog.yaml"),
		path.Join(defaultpaths.ConfPath, "security-agent.yaml"),
	}

	defaultSysProbeConfPath = path.Join(defaultpaths.ConfPath, "system-probe.yaml")
)

// MakeCommand makes the top-level Cobra command for this command.
func MakeCommand(subcommandFactories []SubcommandFactory) *cobra.Command {
	var globalParams GlobalParams

	SecurityAgentCmd := &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
		SilenceUsage: true, // don't print usage on errors
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if globalParams.NoColor {
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
	SecurityAgentCmd.PersistentFlags().BoolVarP(&globalParams.NoColor, "no-color", "n", false, "disable color output")
	SecurityAgentCmd.PersistentFlags().StringVar(&globalParams.FleetPoliciesDirPath, "fleetcfgpath", "", "path to the directory containing fleet policies")
	_ = SecurityAgentCmd.PersistentFlags().MarkHidden("fleetcfgpath")

	for _, factory := range subcommandFactories {
		for _, subcmd := range factory(&globalParams) {
			SecurityAgentCmd.AddCommand(subcmd)
		}
	}

	return SecurityAgentCmd
}
