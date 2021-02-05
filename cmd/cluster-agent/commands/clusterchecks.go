// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver clusterchecks

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func GetClusterChecksCobraCmd(flagNoColor *bool, confPath *string, loggerName config.LoggerName) *cobra.Command {
	clusterChecksCmd := &cobra.Command{
		Use:   "clusterchecks",
		Short: "Prints the active cluster check configurations",
		RunE: func(cmd *cobra.Command, args []string) error {

			if *flagNoColor {
				color.NoColor = true
			}

			// we'll search for a config file named `datadog-cluster.yaml`
			config.Datadog.SetConfigName("datadog-cluster")
			err := common.SetupConfig(*confPath)
			if err != nil {
				return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
			}

			err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			if err = flare.GetClusterChecks(color.Output); err != nil {
				return err
			}

			return flare.GetEndpointsChecks(color.Output)
		},
	}

	return clusterChecksCmd
}

func RebalanceClusterChecksCobraCmd(flagNoColor *bool, confPath *string, loggerName config.LoggerName) *cobra.Command {
	clusterChecksCmd := &cobra.Command{
		Use:   "rebalance",
		Short: "Rebalances cluster checks",
		RunE: func(cmd *cobra.Command, args []string) error {

			if *flagNoColor {
				color.NoColor = true
			}

			// we'll search for a config file named `datadog-cluster.yaml`
			config.Datadog.SetConfigName("datadog-cluster")
			err := common.SetupConfig(*confPath)
			if err != nil {
				return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
			}

			err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
			if err != nil {
				fmt.Printf("Cannot setup logger, exiting: %v\n", err)
				return err
			}

			return rebalanceChecks()
		},
	}

	return clusterChecksCmd
}

func rebalanceChecks() error {
	fmt.Println("Requesting a cluster check rebalance...")
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/api/v1/clusterchecks/rebalance", config.Datadog.GetInt("cluster_agent.cmd_port"))

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
