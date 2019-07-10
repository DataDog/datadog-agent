// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver
// +build clusterchecks

package app

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
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
		// we'll search for a config file named `datadog-cluster.yaml`
		config.Datadog.SetConfigName("datadog-cluster")
		err := common.SetupConfig(confPath)
		if err != nil {
			return fmt.Errorf("unable to set up global cluster agent configuration: %v", err)
		}
		if flagNoColor {
			color.NoColor = true
		}
		err = flare.GetClusterChecks(color.Output)
		if err != nil {
			return err
		}
		err = flare.GetEndpointsChecks(color.Output)
		if err != nil {
			return err
		}
		return nil
	},
}
