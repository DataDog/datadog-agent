// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package app

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	cmdconfig "github.com/DataDog/datadog-agent/cmd/agent/common/commands/config"
	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

func init() {
	SecurityAgentCmd.AddCommand(cmdconfig.Config(getSettingsClient))
}

func setupConfig(cmd *cobra.Command) error {
	if flagNoColor {
		color.NoColor = true
	}

	// Read configuration files received from the command line arguments '-c'
	err := common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed)
	if err != nil {
		return err
	}

	err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	err = util.SetAuthToken()
	if err != nil {
		return err
	}

	return nil
}

func getSettingsClient(cmd *cobra.Command, _ []string) (settings.Client, error) {
	err := setupConfig(cmd)
	if err != nil {
		return nil, err
	}

	c := util.GetClient(false)
	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", config.Datadog.GetInt("security_agent.cmd_port"))

	return settingshttp.NewClient(c, apiConfigURL, "security-agent"), nil
}
