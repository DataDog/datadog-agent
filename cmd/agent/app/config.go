// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	configJSON bool
)

func init() {
	AgentCmd.AddCommand(configCommand)
}

var configCommand = &cobra.Command{
	Use:   "config",
	Short: "Print the runtime configuration of a running agent",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfigWithoutSecrets(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}
		err = util.SetAuthToken()
		if err != nil {
			return err
		}
		if flagNoColor {
			color.NoColor = true
		}

		runtimeConfig, err := requestConfig()
		if err != nil {
			return err
		}

		fmt.Println(runtimeConfig)
		return nil
	},
}

func requestConfig() (string, error) {
	c := util.GetClient(false)
	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", config.Datadog.GetInt("cmd_port"))

	r, err := util.DoGet(c, apiConfigURL)
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap)
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			return "", fmt.Errorf(e)
		}

		return "", fmt.Errorf("Could not reach agent: %v \nMake sure the agent is running before requesting the runtime configuration and contact support if you continue having issues", err)
	}

	return string(r), nil
}
