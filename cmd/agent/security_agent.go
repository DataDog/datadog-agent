// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && bundle_security_agent

// Main package for the agent binary
package main

import (
	seccommand "github.com/DataDog/datadog-agent/cmd/security-agent/command"
	secsubcommands "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/spf13/cobra"
)

func init() {
	registerAgent(func() *cobra.Command {
		flavor.SetFlavor(flavor.SecurityAgent)
		return seccommand.MakeCommand(secsubcommands.SecurityAgentSubcommands())
	}, "security-agent")
}
