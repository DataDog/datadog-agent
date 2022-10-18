// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver
// +build !windows,kubeapiserver

package app

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/app"
	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
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
		// Read configuration files received from the command line arguments '-c'
		return common.MergeConfigurationFiles("datadog-cluster", []string{confPath}, cmd.Flags().Lookup("cfgpath").Changed)
	}

	complianceCmd.AddCommand(checkCmd)
	ClusterAgentCmd.AddCommand(complianceCmd)
}
