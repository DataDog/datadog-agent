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
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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
				ipcfx.ModuleReadOnly(),
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
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	secretInfoCommand.AddCommand(secretRefreshCommand)

	return []*cobra.Command{secretInfoCommand}
}

func showSecretInfo(_ log.Component, client ipc.HTTPClient) error {
	endpoint, err := client.NewIPCEndpoint("/agent/secrets")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return err
	}
	fmt.Println(string(res))
	return nil
}

func secretRefresh(config config.Component, _ log.Component, client ipc.HTTPClient) error {
	fmt.Println("Agent refresh:")
	endpoint, err := client.NewIPCEndpoint("/agent/secret/refresh")
	if err != nil {
		return err
	}
	res, err := endpoint.DoGet()
	if err != nil {
		return err
	}
	fmt.Println(string(res))

	if config.GetBool("apm_config.enabled") {
		fmt.Println("APM agent refresh:")
		res, err = traceAgentSecretRefresh(config, client)
		if err != nil {
			return err
		}
		fmt.Println(string(res))
	}

	{
		fmt.Println("Security Agent refresh:")
		res, err := securityAgentSecretRefresh(config, client)
		if err != nil {
			// the security agent might not be running
			// so we handle the error in a non-fatal way
			fmt.Println(err.Error())
		} else {
			fmt.Println(string(res))
		}
	}

	{
		fmt.Println("Process Agent refresh:")
		res, err := processAgentSecretRefresh(config, client)
		if err != nil {
			// the process agent might not be running
			// so we handle the error in a non-fatal way
			fmt.Println(err.Error())
		} else {
			fmt.Println(string(res))
		}
	}

	return nil
}

func commonSubAgentSecretRefresh(conf config.Component, agentName, portConfigName string, client ipc.HTTPClient) ([]byte, error) {
	port := conf.GetInt(portConfigName)
	if port <= 0 {
		return nil, fmt.Errorf("invalid %s -- %d", portConfigName, port)
	}

	timeout := conf.GetDuration("server_timeout") * time.Second

	url := fmt.Sprintf("https://127.0.0.1:%d/secret/refresh", port)
	res, err := client.Get(url, ipchttp.WithCloseConnection, ipchttp.WithTimeout(timeout))
	if err != nil {
		return nil, fmt.Errorf("could not contact %s: %s", agentName, err)
	}

	return res, nil
}

func traceAgentSecretRefresh(conf config.Component, client ipc.HTTPClient) ([]byte, error) {
	return commonSubAgentSecretRefresh(conf, "trace-agent", "apm_config.debug.port", client)
}

func securityAgentSecretRefresh(conf config.Component, client ipc.HTTPClient) ([]byte, error) {
	return commonSubAgentSecretRefresh(conf, "security-agent", "security_agent.cmd_port", client)
}

func processAgentSecretRefresh(conf config.Component, client ipc.HTTPClient) ([]byte, error) {
	return commonSubAgentSecretRefresh(conf, "process-agent", "process_config.cmd_port", client)
}
