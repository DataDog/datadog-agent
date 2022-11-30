// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadlist implements 'agent workload-list'.
package workloadlist

import (
	"encoding/json"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	verboseList bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	workloadListCommand := &cobra.Command{
		Use:   "workload-list",
		Short: "Print the workload content of a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(workloadList,
				fx.Supply(cliParams),
				fx.Supply(core.CreateAgentBundleParams(globalParams.ConfFilePath, false, core.WithLogForOneShot("CORE", "off", true))),
				core.Bundle,
			)
		},
	}
	workloadListCommand.Flags().BoolVarP(&cliParams.verboseList, "verbose", "v", false, "print out a full dump of the workload store")

	return []*cobra.Command{workloadListCommand}
}

func workloadList(log log.Component, config config.Component, cliParams *cliParams) error {
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	r, err := util.DoGet(c, workloadURL(cliParams.verboseList, ipcAddress, config.GetInt("cmd_port")), util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			fmt.Fprintf(color.Output, "The agent ran into an error while getting the workload store information: %s\n", string(r))
		} else {
			fmt.Fprintf(color.Output, "Failed to query the agent (running?): %s\n", err)
		}
	}

	workload := workloadmeta.WorkloadDumpResponse{}
	err = json.Unmarshal(r, &workload)
	if err != nil {
		return err
	}

	workload.Write(color.Output)

	return nil
}

func workloadURL(verbose bool, address string, port int) string {
	if verbose {
		return fmt.Sprintf("https://%v:%v/agent/workload-list/verbose", address, port)
	}

	return fmt.Sprintf("https://%v:%v/agent/workload-list/short", address, port)
}
