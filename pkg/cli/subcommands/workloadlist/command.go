// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadlist implements 'agent workload-list'.
package workloadlist

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	jsonutil "github.com/DataDog/datadog-agent/pkg/util/json"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams

	args        []string
	verboseList bool
	json        bool
	prettyJSON  bool
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

// MakeCommand returns a `workload-list` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	workloadListCommand := &cobra.Command{
		Use:   "workload-list [search]",
		Short: "Print the workload content of a running agent",
		Long:  ``,
		RunE: func(_ *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			cliParams.GlobalParams = globalParams
			cliParams.args = args

			return fxutil.OneShot(workloadList,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(
						globalParams.ConfFilePath,
						config.WithConfigName(globalParams.ConfigName),
						config.WithExtraConfFiles(globalParams.ExtraConfFilePaths),
						config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
					),
					LogParams: log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(false),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	workloadListCommand.Flags().BoolVarP(&cliParams.verboseList, "verbose", "v", false, "print out a full dump of the workload store (ignored for JSON format)")
	workloadListCommand.Flags().BoolVarP(&cliParams.json, "json", "j", false, "print out raw json")
	workloadListCommand.Flags().BoolVarP(&cliParams.prettyJSON, "pretty-json", "p", false, "pretty print json (takes priority over --json)")

	return workloadListCommand
}

func workloadList(_ log.Component, client ipc.HTTPClient, cliParams *cliParams) error {
	// Validate search argument
	var searchTerm string
	if len(cliParams.args) > 1 {
		return errors.New("only one search term must be specified")
	} else if len(cliParams.args) == 1 {
		searchTerm = cliParams.args[0]
	}

	isJSONFormat := cliParams.json || cliParams.prettyJSON
	url, err := workloadURL(cliParams.verboseList, searchTerm, isJSONFormat)
	if err != nil {
		return err
	}

	r, err := client.Get(url, ipchttp.WithCloseConnection)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while getting the workload store information: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %w", err)
	}

	if isJSONFormat {
		return jsonutil.PrintJSON(color.Output, json.RawMessage(r), cliParams.prettyJSON, true, searchTerm)
	}

	var workload workloadmeta.WorkloadDumpResponse
	err = json.Unmarshal(r, &workload)
	if err != nil {
		return err
	}

	// Check if response is empty when search was provided
	if searchTerm != "" && len(workload.Entities) == 0 {
		return fmt.Errorf("no entities found matching %q", searchTerm)
	}

	workload.Write(color.Output)
	return nil
}

func workloadURL(verbose bool, search string, jsonFormat bool) (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return "", err
	}

	var prefix string
	if flavor.GetFlavor() == flavor.ClusterAgent {
		prefix = fmt.Sprintf("https://%v:%v/workload-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))
	} else {
		prefix = fmt.Sprintf("https://%v:%v/agent/workload-list", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))
	}

	// Build query parameters - backend will process format
	params := url.Values{}
	if verbose {
		params.Set("verbose", "true")
	}
	if search != "" {
		params.Set("search", search)
	}
	if jsonFormat {
		params.Set("format", "json")
	}

	if len(params) > 0 {
		return prefix + "?" + params.Encode(), nil
	}

	return prefix, nil
}
