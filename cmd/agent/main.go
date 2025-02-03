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
	registerAgent([]string{"agent", "datadog-agent", "dd-agent"}, coreAgentMain)
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
	if agentCmdBuilder == nil {
		fmt.Fprintf(os.Stderr, "Invoked as '%s', acting as main Agent.\n", process)
		agentCmdBuilder = coreAgentMain
	}

	rootCmd := agentCmdBuilder()
	if err := setProcessName(process); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set process name as '%s': %s\n", process, err)
	}
	os.Exit(runcmd.Run(rootCmd))
}
