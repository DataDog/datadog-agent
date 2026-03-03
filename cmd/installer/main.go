// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements 'installer'.
package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/cmd/installer/subcommands"
	"github.com/spf13/cobra"
	"go.uber.org/dig"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/commands"
	installerErrors "github.com/DataDog/datadog-agent/pkg/fleet/installer/errors"
)

func main() {
	os.Exit(runCmd(command.MakeCommand(subcommands.InstallerSubcommands())))
}

// formatError returns the error formatted as human-readable text or JSON
// based on the command's annotations.
//
// Most commands are internal and will use JSON errors to communicate result to the parent process.
// The setup command is a special case and will print human-readable errors.
func formatError(cmd *cobra.Command, err error) string {
	if cmd != nil && cmd.Annotations[commands.AnnotationHumanReadableErrors] == "true" {
		return err.Error()
	}
	return installerErrors.ToJSON(err)
}

func runCmd(cmd *cobra.Command) int {
	// always silence errors, since they are handled here
	cmd.SilenceErrors = true

	executedCmd, err := cmd.ExecuteC()
	if err != nil {
		if rootCauseErr := dig.RootCause(err); rootCauseErr != err {
			fmt.Fprintln(cmd.ErrOrStderr(), formatError(executedCmd, rootCauseErr))
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), formatError(executedCmd, err))
		}
		return -1
	}
	return 0
}
