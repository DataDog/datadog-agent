// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterchecks builds a 'clusterchecks' command to be used in binaries.
package clusterchecks

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

const (
	loggerName      = "CLUSTER"
	defaultLogLevel = "off"
)

type GlobalParams struct {
	ConfFilePath string
}

type cliParams struct {
	checkName string
}

// MakeCommand returns a `clusterchecks` command to be used by cluster-agent
// binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "clusterchecks",
		Short: "Prints the active cluster check configurations",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(bundleParams(globalParams)),
				core.Bundle,
			)
		},
	}

	cmd.Flags().StringVarP(&cliParams.checkName, "check", "", "", "the check name to filter for")

	rebalanceCmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalances cluster checks",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			return fxutil.OneShot(rebalance,
				fx.Supply(bundleParams(globalParams)),
				core.Bundle,
			)
		},
	}

	cmd.AddCommand(rebalanceCmd)

	return cmd
}

func bundleParams(globalParams GlobalParams) core.BundleParams {
	return core.BundleParams{
		ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithConfigLoadSecrets(true)),
		LogParams:    log.LogForOneShot(loggerName, defaultLogLevel, true),
	}
}

func run(log log.Component, config config.Component, cliParams *cliParams) error {
	if err := flare.GetClusterChecks(color.Output, cliParams.checkName); err != nil {
		return err
	}

	return flare.GetEndpointsChecks(color.Output, cliParams.checkName)
}

func rebalance(log log.Component, config config.Component) error {
	fmt.Println("Requesting a cluster check rebalance...")
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/clusterchecks/rebalance", pkgconfig.Datadog.GetInt("cluster_agent.cmd_port"))

	// Set session token
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	r, err := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer([]byte{}))
	if err != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if e, found := errMap["error"]; found {
			err = fmt.Errorf(e)
		}

		fmt.Printf(`
		Could not reach agent: %v
		Make sure the agent is running before requesting the cluster checks rebalancing.
		Contact support if you continue having issues.`, err)

		return err
	}

	checksMoved := make([]types.RebalanceResponse, 0)
	json.Unmarshal(r, &checksMoved) //nolint:errcheck

	fmt.Printf("%d cluster checks rebalanced successfully\n", len(checksMoved))

	for _, check := range checksMoved {
		fmt.Printf("Check %s with weight %d moved from node %s to %s. source diff: %d, dest diff: %d\n",
			check.CheckID, check.CheckWeight, check.SourceNodeName, check.DestNodeName, check.SourceDiff, check.DestDiff)
	}

	return nil
}
