// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secret implements 'agent secret'.
package secret

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
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

func showSecretInfo(config config.Component) error {
	r, err := callAPIEndpoint("/agent/secrets", config)
	if err != nil {
		return err
	}
	fmt.Println(string(r))
	return nil
}

func secretRefresh(config config.Component) error {
	r, err := callAPIEndpoint("/agent/secret/refresh", config)
	if err != nil {
		return err
	}
	fmt.Println(string(r))
	return nil
}

func callAPIEndpoint(apiEndpointPath string, config config.Component) ([]byte, error) {
	if err := util.SetAuthToken(config); err != nil {
		fmt.Println(err)
		return nil, err
	}
	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return nil, err
	}
	apiConfigURL := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s:%d", ipcAddress, config.GetInt("cmd_port")),
		Path:   apiEndpointPath,
	}

	r, err := util.DoGet(c, apiConfigURL.String(), util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return nil, fmt.Errorf("%s", e)
		}

		return nil, fmt.Errorf("Could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}
	return r, nil
}
