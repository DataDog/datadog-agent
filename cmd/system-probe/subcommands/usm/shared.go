// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	sysconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// makeOneShotCommand creates a USM command that runs with FX OneShot pattern.
// This eliminates boilerplate for creating USM subcommands.
func makeOneShotCommand(
	globalParams *command.GlobalParams,
	use string,
	short string,
	runFunc interface{},
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runFunc,
				fx.Supply(globalParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.DatadogConfFilePath()),
					SysprobeConfigParams: sysconfigimpl.NewParams(
						sysconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath),
						sysconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams: log.ForOneShot(command.LoggerName, "off", false),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
		SilenceUsage: true,
	}

	return cmd
}
