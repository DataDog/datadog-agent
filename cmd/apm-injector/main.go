// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package main implements 'apm-injector'.
package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/apm-injector/command"
	"github.com/DataDog/datadog-agent/cmd/apm-injector/subcommands"
	"github.com/spf13/cobra"
)

func main() {
	os.Exit(runCmd(command.MakeCommand(subcommands.APMInjectorSubcommands())))
}

func runCmd(cmd *cobra.Command) int {
	// always silence errors, since they are handled here
	cmd.SilenceErrors = true

	_, err := cmd.ExecuteC()
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
		return 1
	}
	return 0
}
