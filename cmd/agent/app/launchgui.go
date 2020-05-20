// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/spf13/cobra"
)

var (
	launchCmd = &cobra.Command{
		Use:          "launch-gui",
		Short:        "starts the Datadog Agent GUI",
		Long:         ``,
		RunE:         launchGui,
		SilenceUsage: true,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(launchCmd)

}

func launchGui(cmd *cobra.Command, args []string) error {
	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	guiPort := config.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// Read the authentication token: can only be done if user can read from datadog.yaml
	authToken, err := security.FetchAuthToken()
	if err != nil {
		return err
	}

	// Get the CSRF token from the agent
	c := util.GetClient(false) // FIX: get certificates right then make this true
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	urlstr := fmt.Sprintf("https://%v:%v/agent/gui/csrf-token", ipcAddress, config.Datadog.GetInt("cmd_port"))
	err = util.SetAuthToken()
	if err != nil {
		return err
	}

	csrfToken, err := util.DoGet(c, urlstr)
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
	err = open("http://127.0.0.1:" + guiPort + "/authenticate?authToken=" + authToken + ";csrf=" + string(csrfToken))
	if err != nil {
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	fmt.Printf("GUI opened at 127.0.0.1:" + guiPort + "\n")
	return nil
}
