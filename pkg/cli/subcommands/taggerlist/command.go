// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taggerlist implements 'agent tagger-list'.
package taggerlist

import (
	"errors"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/api"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams
	args       []string
	json       bool
	prettyJSON bool
}

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath         string
	ExtraConfFilePaths   []string
	ConfigName           string
	LoggerName           string
	FleetPoliciesDirPath string
}

// MakeCommand returns a `tagger-list` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "tagger-list [search]",
		Short: "Print the tagger content of a running agent",
		Long:  ``,
		RunE: func(_ *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			cliParams.GlobalParams = globalParams
			cliParams.args = args

			return fxutil.OneShot(taggerList,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(
						globalParams.ConfFilePath,
						config.WithConfigName(globalParams.ConfigName),
						config.WithExtraConfFiles(globalParams.ExtraConfFilePaths),
						config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
					),
					LogParams: log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cmd.Flags().BoolVarP(&cliParams.json, "json", "j", false, "print out raw json")
	cmd.Flags().BoolVarP(&cliParams.prettyJSON, "pretty-json", "p", false, "pretty print json (takes priority over --json)")

	return cmd
}

func taggerList(_ log.Component, config config.Component, client ipc.HTTPClient, cliParams *cliParams) error {
	url, err := getTaggerURL(config)
	if err != nil {
		return err
	}

	// Validate search argument
	var searchTerm string
	if len(cliParams.args) > 1 {
		return errors.New("only one search term must be specified")
	} else if len(cliParams.args) == 1 {
		searchTerm = cliParams.args[0]
	}

	return api.GetTaggerList(client, color.Output, url, cliParams.json, cliParams.prettyJSON, searchTerm)
}

func getTaggerURL(config config.Component) (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	var urlstr string
	if flavor.GetFlavor() == flavor.ClusterAgent {
		urlstr = fmt.Sprintf("https://%v:%v/tagger-list", ipcAddress, config.GetInt("cluster_agent.cmd_port"))
	} else {
		urlstr = fmt.Sprintf("https://%v:%v/agent/tagger-list", ipcAddress, config.GetInt("cmd_port"))
	}

	return urlstr, nil
}
