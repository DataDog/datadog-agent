// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const taggerListURLTpl = "http://%s/agent/tagger-list"

// TaggerCmd is a command that prints the process-agent version data
var TaggerCmd = &cobra.Command{
	Use:   "tagger-list",
	Short: "Print the tagger content of a running agent",
	Long:  "",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getTaggerList(cmd)
	},
	SilenceUsage: true,
}

func getTaggerList(cmd *cobra.Command) error {
	log.Info("Got a request for the tagger-list. Calling tagger.")

	if err := initConfig(os.Stdout, cmd); err != nil {
		return err
	}

	taggerURL, err := getTaggerURL()
	if err != nil {
		return err
	}

	return tagger_api.GetTaggerList(color.Output, taggerURL)
}

func getTaggerURL() (string, error) {
	addressPort, err := ddconfig.GetProcessAPIAddressPort()
	if err != nil {
		return "", fmt.Errorf("config error: %s", err.Error())
	}
	return fmt.Sprintf(taggerListURLTpl, addressPort), nil
}
