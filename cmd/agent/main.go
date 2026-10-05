// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Main package for the agent binary
package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/remotecommand"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/spf13/cobra"
)

var agents = map[string]func() *cobra.Command{}

func registerAgent(names []string, getCommand func() *cobra.Command) {
	for _, name := range names {
		agents[name] = getCommand
	}
}

func coreAgentMain() *cobra.Command {
	return command.MakeCommand(subcommands.AgentSubcommands())
}

func init() {
	// "agent-bin" is the core Agent binary name in the AIX installp package
	// (the "agent" entry point there is a wrapper script that execs agent-bin
	// with LIBPATH/NLSPATH set, see packaging/aix/agent-wrapper.sh).
	registerAgent([]string{"agent", "datadog-agent", "dd-agent", "agent-bin"}, coreAgentMain)
}

func main() {
	process := strings.TrimSpace(os.Getenv("DD_BUNDLED_AGENT"))

	if process == "" {
		if len(os.Args) > 0 {
			process = strings.TrimSpace(path.Base(os.Args[0]))
		}

		if process == "" {
			executable, err := os.Executable()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to determine the Agent process name: %s\n", err.Error())
				os.Exit(1)
			}
			process = executable
		}

		process = strings.TrimSuffix(process, path.Ext(process))
	}

	agentCmdBuilder := agents[process]
	// The core Agent process names below construct the Core Agent command tree and must
	// run RemoteCommandProvider discovery; other bundled binaries must not perform it
	// before their own Cobra dispatch.
	isCoreAgent := agentCmdBuilder == nil || process == "agent" || process == "datadog-agent" || process == "dd-agent" || process == "agent-bin"
	if agentCmdBuilder == nil {
		fmt.Fprintf(os.Stderr, "Invoked as '%s', acting as main Agent.\n", process)
		agentCmdBuilder = coreAgentMain
	}

	rootCmd := agentCmdBuilder()
	if isCoreAgent {
		if err := remotecommand.Prepare(rootCmd, os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if err := setProcessName(process); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set process name as '%s': %s\n", process, err)
	}
	os.Exit(runcmd.Run(rootCmd))
}
