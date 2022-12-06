// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/security-agent/commands/config"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

func Commands(globalParams *common.GlobalParams) []*cobra.Command {
	cmd := cmdconfig.Config(getSettingsClient)
	return []*cobra.Command{cmd}
}

func setupConfig(cmd *cobra.Command) error {
	err := config.SetupLogger(common.LoggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	return util.SetAuthToken()
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
