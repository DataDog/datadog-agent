// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package purge implements 'installer purge'.
package purge

import (
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/installer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	command.GlobalParams
	pkg string
}

// Commands returns the run command
func Commands(global *command.GlobalParams) []*cobra.Command {
	var pkg string
	purgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Purge installer packages",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return purgeFxWrapper(&cliParams{
				GlobalParams: *global,
				pkg:          pkg,
			})
		},
	}
	purgeCmd.Flags().StringVarP(&pkg, "package", "P", "", "Package name to purge")
	return []*cobra.Command{purgeCmd}
}

func purgeFxWrapper(params *cliParams) error {
	return fxutil.OneShot(purge,
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			LogParams: logimpl.ForOneShot("INSTALLER", "info", true),
		}),
		core.Bundle(),
	)
}

func purge(params *cliParams) error {
	if params.pkg != "" {
		installer.PurgePackage(params.pkg)
	} else {
		installer.Purge()
	}
	return nil
}
