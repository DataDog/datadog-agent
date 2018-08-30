// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver
// +build clusterchecks

package app

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/flare"
)

func init() {
	ClusterAgentCmd.AddCommand(clusterChecksCmd)
}

var clusterChecksCmd = &cobra.Command{
	Use:   "clusterchecks",
	Short: "Prints the active cluster check configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		err := common.SetupConfig(confPath)
		if err != nil {
			return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
		}
		return getClusterChecks()
	},
}

func getClusterChecks() error {
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true
	urlstr := fmt.Sprintf("https://localhost:%v/api/v2beta1/clusterchecks", config.Datadog.GetInt("cluster_agent.cmd_port"))

	// Set session token
	e = util.SetAuthToken()
	if e != nil {
		return e
	}

	r, e := util.DoGet(c, urlstr)
	if e != nil {
		fmt.Printf(`
		Could not reach agent: %v
		Make sure the agent is properly running before requesting the map of services to pods.
		Contact support if you continue having issues.`, e)
		return e
	}

	configs := []integration.Config{}
	e = json.Unmarshal(r, &configs)
	if e != nil {
		return e
	}

	if len(configs) == 0 {
		fmt.Printf("No cluster-check configuration\n")
	} else {
		fmt.Printf("Retrieved %d cluster-check configurations:\n", len(configs))

	}
	for _, c := range configs {
		flare.PrintConfig(os.Stdout, c)
	}

	return nil
}
