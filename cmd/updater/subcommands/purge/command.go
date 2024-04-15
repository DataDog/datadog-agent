// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package purge implements 'installer purge'.
package purge

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns the run command
func Commands(_ *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge installer packages",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return purgeFxWrapper()
		},
	}
	return []*cobra.Command{runCmd}
}

func purgeFxWrapper() error {
	return fxutil.OneShot(purge,
		fx.Supply(core.BundleParams{
			LogParams: logimpl.ForOneShot("UPDATER", "info", true),
		}),
		core.Bundle(),
		telemetryimpl.Module(),
	)
}

func purge(_ telemetry.Component) error {
	installer.Purge()
	return nil
}
