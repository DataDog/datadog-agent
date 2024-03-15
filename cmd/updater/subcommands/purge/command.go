// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package purge implements 'updater purge'.
package purge

import (
	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

// Commands returns the run command
func Commands(_ *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge updater packages",
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
	)
}

func purge() error {
	updater.Purge()
	return nil
}
