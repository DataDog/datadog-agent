// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent configcheck'.
package configcheck

import (
	"bytes"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	verbose bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	configCheckCommand := &cobra.Command{
		Use:     "configcheck",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running agent",
		Long:    ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    logimpl.ForOneShot("CORE", "off", true)}),
				core.Bundle(),
			)
		},
	}
	configCheckCommand.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")

	return []*cobra.Command{configCheckCommand}
}

func run(_ config.Component, cliParams *cliParams) error {
	var b bytes.Buffer
	color.Output = &b
	err := flare.GetConfigCheck(color.Output, cliParams.verbose)
	if err != nil {
		return fmt.Errorf("unable to get pkgconfig: %v", err)
	}

	fmt.Println(b.String())
	return nil
}
