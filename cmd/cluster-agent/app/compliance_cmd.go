// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	complianceCmd = &cobra.Command{
		Use:   "compliance",
		Short: "Compliance utility commands",
	}
)

func init() {
	checkCmd := app.CheckCmd()
	checkCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		// we'll search for a config file named `datadog-cluster.yaml`
		config.Datadog.SetConfigName("datadog-cluster")
		err := common.SetupConfig(confPath)
		if err != nil {
			return fmt.Errorf("unable to set up global cluster agent configuration: %w", err)
		}
		return nil
	}

	complianceCmd.AddCommand(checkCmd)
	ClusterAgentCmd.AddCommand(complianceCmd)
}
