// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package check holds check related files
package check

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/compliance/cli"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// ClusterAgentCommands returns the cluster agent commands
func ClusterAgentCommands(bundleParams core.BundleParams) []*cobra.Command {
	return commandsWrapped(func() core.BundleParams {
		return bundleParams
	})
}

func commandsWrapped(bundleParamsFactory func() core.BundleParams) []*cobra.Command {
	checkArgs := &cli.CheckParams{}

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run compliance check(s)",
		Long:  ``,
		RunE: func(_ *cobra.Command, args []string) error {
			checkArgs.Args = args

			bundleParams := bundleParamsFactory()
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
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cli.FillCheckFlags(cmd.Flags(), checkArgs)

	return []*cobra.Command{cmd}
}
