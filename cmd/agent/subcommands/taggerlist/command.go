// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taggerlist implements 'agent tagger-list'.
package taggerlist

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
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
	taggerListCommand := &cobra.Command{
		Use:   "tagger-list",
		Short: "Print the tagger content of a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(taggerList,
				fx.Supply(cliParams),
				fx.Supply(core.CreateAgentBundleParams(globalParams.ConfFilePath, false, core.WithLogForOneShot("CORE", "off", true))),
				core.Bundle,
			)
		},
	}

	return []*cobra.Command{taggerListCommand}
}

func taggerList(log log.Component, config config.Component, cliParams *cliParams) error {
	// Set session token
	if err := util.SetAuthToken(); err != nil {
		return err
	}

	url, err := getTaggerURL(config)
	if err != nil {
		return err
	}

	return tagger_api.GetTaggerList(color.Output, url)
}

func getTaggerURL(config config.Component) (string, error) {
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.GetInt("cmd_port")), nil
}
