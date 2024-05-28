// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package launchgui implements 'agent launch-gui'.
package launchgui

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
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
	launchCmd := &cobra.Command{
		Use:   "launch-gui",
		Short: "starts the Datadog Agent GUI",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(launchGui,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath)}),
				core.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	return []*cobra.Command{launchCmd}
}

func launchGui(config config.Component, _ *cliParams) error {
	guiPort := pkgconfig.Datadog().GetString("GUI_port")
	if guiPort == "-1" {
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// Read the authentication token: can only be done if user can read from datadog.yaml
	authToken, err := security.FetchAuthToken(config)
	if err != nil {
		return err
	}

	// Get the CSRF token from the agent
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/gui/csrf-token", ipcAddress, pkgconfig.Datadog().GetInt("cmd_port"))
	err = util.SetAuthToken(config)
	if err != nil {
		return err
	}

	csrfToken, err := util.DoGet(c, urlstr, util.LeaveConnectionOpen)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(csrfToken, &errMap) //nolint:errcheck
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}
		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before attempting to open the GUI.\n", err)
		return err
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://127.0.0.1:" + guiPort + "/authenticate?authToken=" + authToken + "&csrf=" + string(csrfToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	fmt.Printf("GUI opened at 127.0.0.1:" + guiPort + "\n")
	return nil
}
