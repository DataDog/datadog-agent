// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taggerlist implements 'agent tagger-list'.
package taggerlist

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	taggerListCommand := &cobra.Command{
		Use:   "tagger-list",
		Short: "Print the tagger content of a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := common.SetupConfigWithoutSecrets(globalParams.ConfFilePath, "")
			if err != nil {
				return fmt.Errorf("unable to set up global agent configuration: %v", err)
			}

			err = config.SetupLogger(config.CoreLoggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			// Set session token
			if err := util.SetAuthToken(); err != nil {
				return err
			}

			url, err := getTaggerURL()
			if err != nil {
				return err
			}

			return tagger_api.GetTaggerList(color.Output, url)
		},
	}

	return []*cobra.Command{taggerListCommand}
}

func getTaggerURL() (string, error) {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.Datadog.GetInt("cmd_port")), nil
}
