// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secret implements 'agent secret'.
package secret

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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
		RunE: func(_ *cobra.Command, _ []string) error {
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
		RunE: func(_ *cobra.Command, _ []string) error {
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
	fmt.Println("Agent refresh:")
	res, err := callIPCEndpoint(config, "agent/secret/refresh")
	if err != nil {
		return err
	}
	fmt.Println(string(res))

	if config.GetBool("apm_config.enabled") {
		fmt.Println("APM agent refresh:")
		res, err = traceAgentSecretRefresh(config)
		if err != nil {
			return err
		}
		fmt.Println(string(res))
	}
	return nil
}

func traceAgentSecretRefresh(conf config.Component) ([]byte, error) {
	err := apiutil.SetAuthToken(conf)
	if err != nil {
		return nil, err
	}

	port := conf.GetInt("apm_config.debug.port")
	if port <= 0 {
		return nil, fmt.Errorf("invalid apm_config.debug.port -- %d", port)
	}

	c := apiutil.GetClient(false)
	c.Timeout = conf.GetDuration("server_timeout") * time.Second

	url := fmt.Sprintf("http://127.0.0.1:%d/secret/refresh", port)
	res, err := apiutil.DoGet(c, url, apiutil.CloseConnection)
	if err != nil {
		return nil, fmt.Errorf("could not contact trace-agent: %s", err)
	}

	return res, nil
}

func callIPCEndpoint(config config.Component, endpointURL string) ([]byte, error) {
	endpoint, err := apiutil.NewIPCEndpoint(config, endpointURL)
	if err != nil {
		return nil, err
	}
	return endpoint.DoGet()
}
