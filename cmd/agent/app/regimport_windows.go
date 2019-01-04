// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/spf13/cobra"
)

var (
	regImportCmd = &cobra.Command{
		Use:   "regimport",
		Short: "Import the registry settings into datadog.yaml",
		Long:  ``,
		RunE:  doRegImport,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(regImportCmd)

}

func doRegImport(cmd *cobra.Command, args []string) error {
	return common.ImportRegistryConfig()
}
