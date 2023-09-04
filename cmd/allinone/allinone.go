// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Main package for the allinone binary
package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	processcommand "github.com/DataDog/datadog-agent/cmd/process-agent/command"
	processsubcommands "github.com/DataDog/datadog-agent/cmd/process-agent/subcommands"
	seccommand "github.com/DataDog/datadog-agent/cmd/security-agent/command"
	secsubcommands "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands"
	sysprobecommand "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysprobesubcommands "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands"
	tracecommand "github.com/DataDog/datadog-agent/cmd/trace-agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd *cobra.Command

	executable := path.Base(os.Args[0])
	process := strings.TrimSuffix(executable, path.Ext(executable))

	switch process {
	case "process-agent":
		flavor.SetFlavor(flavor.ProcessAgent)
		os.Args = processcommand.FixDeprecatedFlags(os.Args, os.Stdout)
		rootCmd = processcommand.MakeCommand(processsubcommands.ProcessAgentSubcommands(), processcommand.UseWinParams, processcommand.RootCmdRun)
	case "security-agent":
		flavor.SetFlavor(flavor.SecurityAgent)
		rootCmd = seccommand.MakeCommand(secsubcommands.SecurityAgentSubcommands())
	case "trace-agent":
		defaultLogFile := "/var/log/datadog/trace-agent.log"
		os.Args = tracecommand.FixDeprecatedFlags(os.Args, os.Stdout)
		rootCmd = tracecommand.MakeRootCommand(defaultLogFile)
	case "agent", "datadog-agent":
		rootCmd = command.MakeCommand(subcommands.AgentSubcommands())
	case "system-probe":
		rootCmd = sysprobecommand.MakeCommand(sysprobesubcommands.SysprobeSubcommands())
		sysprobecommand.SetDefaultCommandIfNonePresent(rootCmd)
	default:
		fmt.Fprintf(os.Stderr, "'%s' is an incorrect invocation of the Datadog Agent\n", process)
	}

	os.Exit(runcmd.Run(rootCmd))
}
