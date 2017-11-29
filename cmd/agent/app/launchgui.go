// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
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
	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	guiPort := config.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		log.Warnf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
		return fmt.Errorf("GUI not enabled: to enable, please set an appropriate port in your datadog.yaml file")
	}

	// Read the authentication token: can only be done if user can read from datadog.yaml
	authToken, err := ioutil.ReadFile(filepath.Join(filepath.Dir(config.Datadog.ConfigFileUsed()), "gui_auth_token"))
	if err != nil {
		return fmt.Errorf("unable to access GUI authentication token: " + err.Error())
	}

	// Open the GUI in a browser, passing the authorization tokens as parameters
	err = open("http://127.0.0.1:" + guiPort + string(authToken))
	if err != nil {
		log.Warnf("error opening GUI: " + err.Error())
		return fmt.Errorf("error opening GUI: " + err.Error())
	}

	log.Infof("GUI opened at 127.0.0.1:" + guiPort)
	return nil
}
