// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/pkg/security/embedtests"
	"github.com/spf13/cobra"
)

var (
	runtimeEmbededTestsCmd = &cobra.Command{
		Use:   "ret",
		Short: "Run the CWS embeded tests",
		Long:  `Runs the CWS embeded tests`,
		RunE:  runtimeRunEmbededTests,
	}
)

func init() {
	// attach the command to the root
	SysprobeCmd.AddCommand(runtimeEmbededTestsCmd)
}

func runtimeRunEmbededTests(cmd *cobra.Command, args []string) error {
	embedtests.RunEmbedTests()
	return nil
}
