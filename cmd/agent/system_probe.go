// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && bundle_system_probe

// Main package for the agent binary
package main

import (
	sysprobecommand "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysprobesubcommands "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands"
	"github.com/spf13/cobra"
)

func init() {
	registerAgent([]string{"system-probe"}, func() *cobra.Command {
		rootCmd := sysprobecommand.MakeCommand(sysprobesubcommands.SysprobeSubcommands())
		sysprobecommand.SetDefaultCommandIfNonePresent(rootCmd)
		return rootCmd
	})
}
