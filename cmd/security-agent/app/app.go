// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"path"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	commonagent "github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/compliance"
	subconfig "github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/config"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/flare"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/start"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/status"
	subversion "github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/version"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func CreateSecurityAgentCmd() *cobra.Command {
	globalParams := common.GlobalParams{}
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

			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			log.Flush()
		},
	}

	defaultConfPathArray := []string{
		path.Join(commonagent.DefaultConfPath, "datadog.yaml"),
		path.Join(commonagent.DefaultConfPath, "security-agent.yaml"),
	}
	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&globalParams.ConfPathArray, "cfgpath", "c", defaultConfPathArray, "path to a yaml configuration file")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	factories := []common.SubcommandFactory{
		status.Commands,
		flare.Commands,
		subconfig.Commands,
		compliance.Commands,
		runtime.Commands,
		subversion.Commands,
		start.Commands,
	}

	for _, factory := range factories {
		for _, subcmd := range factory(&globalParams) {
			SecurityAgentCmd.AddCommand(subcmd)
		}
	}

	return SecurityAgentCmd
}
