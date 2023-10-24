// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !no_agent

// Main package for the allinone binary
package main

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/spf13/cobra"
)

func init() {
	registerAgent(func() *cobra.Command {
		return command.MakeCommand(subcommands.AgentSubcommands())
	}, "agent", "datadog-agent")
}
