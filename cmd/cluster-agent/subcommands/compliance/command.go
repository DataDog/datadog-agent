// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package compliance implements 'cluster-agent compliance'.
package compliance

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/compliance/cli"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	complianceCmd := &cobra.Command{
		Use:   "compliance",
		Short: "compliance utility commands",
	}

	complianceCmd.AddCommand(complianceCheckCommand(globalParams))

	return []*cobra.Command{complianceCmd}
}

func complianceCheckCommand(globalParams *command.GlobalParams) *cobra.Command {
	checkArgs := &cli.CheckParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		RunE: func(_ *cobra.Command, args []string) error {
			bundleParams := core.BundleParams{
				ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
				LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
			}

			checkArgs.Args = args
			if checkArgs.Verbose {
				bundleParams.LogParams = log.ForOneShot(bundleParams.LogParams.LoggerName(), "trace", true)
			}

			return fxutil.OneShot(cli.RunCheck,
				fx.Supply(checkArgs),
				fx.Supply(bundleParams),
				core.Bundle(core.WithSecrets()),
				logscompressionfx.Module(),
				statsd.Module(),
				ipcfx.ModuleReadOnly(),
				hostnameimpl.Module(),
			)
		},
	}

	cli.FillCheckFlags(cmd.Flags(), checkArgs)

	return cmd
}
