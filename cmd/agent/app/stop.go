// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build !windows

package app

import (
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/spf13/cobra"
)

var (
	stopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop the Agent",
		Long:  ``,
		Run:   stop,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(stopCmd)
}

func stop(*cobra.Command, []string) {
	// Global Agent configuration
	common.SetupConfig("")
	// get an API client
	signals.Stopper <- true
}
