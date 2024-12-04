// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !bootstrapper

// Package main implements 'installer'.
package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands"
	"github.com/spf13/cobra"
	"go.uber.org/dig"

	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
)

func main() {
	os.Exit(runCmd(command.MakeCommand(subcommands.InstallerSubcommands())))
}

func runCmd(cmd *cobra.Command) int {
	// always silence errors, since they are handled here
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err != nil {
		if rootCauseErr := dig.RootCause(err); rootCauseErr != err {
			fmt.Fprintln(cmd.ErrOrStderr(), installerErrors.FromErr(rootCauseErr).ToJSON())
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), installerErrors.FromErr(err).ToJSON())
		}
		return -1
	}
	return 0
}
