// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build bundle_installer && !windows && !darwin

package main

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands"
)

func init() {
	registerAgent([]string{"datadog-installer", "installer"}, func() *cobra.Command {
		return command.MakeCommand(subcommands.InstallerSubcommands())
	})
}
