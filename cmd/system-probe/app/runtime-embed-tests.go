// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"github.com/DataDog/datadog-agent/pkg/security/embeddedtests"
	"github.com/spf13/cobra"
)

var (
	runtimeEmbeddedTestsCmd = &cobra.Command{
		Use:   "runtime-embedded-tests",
		Short: "Run the CWS embedded tests",
		Long:  `Runs the CWS embedded tests`,
		Run:   runtimeRunEmbeddedTests,
	}
)

func init() {
	// attach the command to the root
	SysprobeCmd.AddCommand(runtimeEmbeddedTestsCmd)
}

func runtimeRunEmbeddedTests(cmd *cobra.Command, args []string) {
	embeddedtests.RunEmbeddedTests()
}
