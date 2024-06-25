// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secret implements 'agent secret'.
package secret

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	secretInfoCommand := &cobra.Command{
		Use:   "secret",
		Short: "Display information about secrets in configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(showSecretInfo,
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}
	secretRefreshCommand := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh secrets in configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(secretRefresh,
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}
	secretInfoCommand.AddCommand(secretRefreshCommand)

	return []*cobra.Command{secretInfoCommand}
}

func showSecretInfo(config config.Component, _ log.Component) error {
	res, err := callIPCEndpoint(config, "agent/secrets")
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func secretRefresh(config config.Component, _ log.Component) error {
	res, err := callIPCEndpoint(config, "agent/secret/refresh")
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func callIPCEndpoint(config config.Component, endpointURL string) ([]byte, error) {
	endpoint, err := apiutil.NewIPCEndpoint(config, endpointURL)
	if err != nil {
		return nil, err
	}
	return endpoint.DoGet()
}
