// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package compliance implements compliance related subcommands
package compliance

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/compliance/cli"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns the compliance commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "Compliance Agent utility commands",
	}

	complianceCmd.AddCommand(CheckCommand(globalParams))
	complianceCmd.AddCommand(complianceLoadCommand(globalParams))
	addPlatformSpecificCommands(complianceCmd, globalParams)

	return []*cobra.Command{complianceCmd}
}

// CheckCommand returns the 'compliance check' command
func CheckCommand(globalParams *command.GlobalParams) *cobra.Command {
	checkArgs := &cli.CheckParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		RunE: func(_ *cobra.Command, args []string) error {
			bundleParams := core.BundleParams{
				ConfigParams:         config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
				LogParams:            log.ForOneShot(command.LoggerName, "info", true),
			}

			checkArgs.Args = args
			if checkArgs.Verbose {
				bundleParams.LogParams = log.ForOneShot(bundleParams.LogParams.LoggerName(), "trace", true)
			}

			return fxutil.OneShot(cli.RunCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle(),
				secretsfx.Module(),
				logscompressionfx.Module(),
				statsd.Module(),
				ipcfx.ModuleInsecure(),
				remotehostnameimpl.Module(),
			)
		},
	}

	cli.FillCheckFlags(cmd.Flags(), checkArgs)

	return cmd
}

func complianceLoadCommand(globalParams *command.GlobalParams) *cobra.Command {
	loadArgs := &cli.LoadParams{}

	loadCmd := &cobra.Command{
		Use:   "load <conf-type>",
		Short: "Load compliance config",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			loadArgs.ConfType = args[0]
			return fxutil.OneShot(cli.RunLoad,
				fx.Supply(loadArgs),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}

	cli.FillLoadFlags(loadCmd.Flags(), loadArgs)

	return loadCmd
}
