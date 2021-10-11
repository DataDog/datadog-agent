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
	cwsEmbeddedTestsCmd = &cobra.Command{
		Use:   "cws-tests",
		Short: "Run the CWS embedded tests",
		Long:  `Run the CWS embedded tests`,
		Run:   runCWSEmbeddedTests,
	}
)

func init() {
	// attach the command to the root
	SysprobeCmd.AddCommand(cwsEmbeddedTestsCmd)
}

func runCWSEmbeddedTests(cmd *cobra.Command, args []string) {
	embeddedtests.RunEmbeddedTests()
}
