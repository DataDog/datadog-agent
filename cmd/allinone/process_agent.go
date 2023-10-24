// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !no_process_agent

// Main package for the allinone binary
package main

import (
	"os"

	processcommand "github.com/DataDog/datadog-agent/cmd/process-agent/command"
	processsubcommands "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/spf13/cobra"
)

func init() {
	registerAgent(func() *cobra.Command {
		flavor.SetFlavor(flavor.ProcessAgent)
		os.Args = processcommand.FixDeprecatedFlags(os.Args, os.Stdout)
		return processcommand.MakeCommand(processsubcommands.ProcessAgentSubcommands(), processcommand.UseWinParams, processcommand.RootCmdRun)
	}, "process-agent")
}
