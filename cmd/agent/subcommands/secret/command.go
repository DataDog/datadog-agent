// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secret implements 'agent secret'.
package secret

import (
	"encoding/json"
	"fmt"
	"os"

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

	// args are the positional command-line arguments
	args []string

	// cmd is the cobra Command, used to show help
	cmd *cobra.Command
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "secret [refresh]",
		Short: "Display information about or refresh secrets in configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.cmd = cmd
			cliParams.args = args
			return fxutil.OneShot(secretCommand,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{cmd}
}

func secretCommand(config config.Component, cliParams *cliParams) error {
	if err := util.SetAuthToken(); err != nil {
		fmt.Println(err)
		return nil
	}

	if len(cliParams.args) == 0 {
		if err := showSecretInfo(config); err != nil {
			fmt.Println(err)
		}
	} else if len(cliParams.args) == 1 && cliParams.args[0] == "refresh" {
		if err := secretRefresh(config); err != nil {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("The command arguments must be empty or 'refresh'\n")
		cliParams.cmd.Help() //nolint:errcheck
		os.Exit(1)
	}

	return nil
}

func showSecretInfo(config config.Component) error {
	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%v/agent/secrets", ipcAddress, config.GetInt("cmd_port"))

	r, err := util.DoGet(c, apiConfigURL, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return fmt.Errorf("%s", e)
		}

		return fmt.Errorf("Could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}

	fmt.Println(string(r))
	return nil
}

func secretRefresh(config config.Component) error {
	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%v/agent/secret/refresh", ipcAddress, config.GetInt("cmd_port"))

	r, err := util.DoGet(c, apiConfigURL, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return fmt.Errorf("%s", e)
		}

		return fmt.Errorf("Could not reach agent: %v\nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}

	fmt.Printf("Secrets refresh: %s\n", r)
	return nil
}
