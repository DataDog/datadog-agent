// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package autoscalerlist implements 'agent autoscaler-list'.
package autoscalerlist

import (
	"encoding/json"
	"fmt"
	"io"

	"go.uber.org/fx"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	autoscalingWorkload "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams
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

// MakeCommand returns an`autoscaler-list` command to be used by cluster- binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	return &cobra.Command{
		Use:   "autoscaler-list",
		Short: "Print the autoscaling store content of a running agent",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalParamsGetter()

			cliParams.GlobalParams = globalParams

			return fxutil.OneShot(autoscalerList,
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
			)
		},
	}
}

func autoscalerList(_ log.Component, config config.Component, _ *cliParams) error {
	// Set session token
	if err := util.SetAuthToken(config); err != nil {
		return err
	}

	url, err := getAutoscalerURL(config)
	if err != nil {
		return err
	}

	return getAutoscalerList(color.Output, url)
}

func getAutoscalerURL(config config.Component) (string, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return "", err
	}

	var urlstr string
	if flavor.GetFlavor() == flavor.ClusterAgent {
		urlstr = fmt.Sprintf("https://%v:%v/autoscaler-list", ipcAddress, config.GetInt("cluster_agent.cmd_port"))
	} else {
		return "", fmt.Errorf("running autoscaler-list is only supported on the cluster agent")
	}

	return urlstr, nil
}

func getAutoscalerList(w io.Writer, url string) error {
	c := util.GetClient()

	// get the autoscaler-list from server
	r, err := util.DoGet(c, url, util.LeaveConnectionOpen)
	if err != nil {
		if r != nil && string(r) != "" {
			return fmt.Errorf("the agent ran into an error while getting autoscaler list: %s", string(r))
		}
		return fmt.Errorf("failed to query the agent (running?): %s", err)
	}

	if len(r) == 0 {
		return fmt.Errorf("no autoscalers found")
	}

	adr := autoscalingWorkload.AutoscalingDumpResponse{}
	if err = json.Unmarshal(r, &adr); err != nil {
		return fmt.Errorf("error unmarshalling json: %s", err)
	}

	adr.Write(w)
	return nil
}
