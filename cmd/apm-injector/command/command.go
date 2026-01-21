// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package command implements the top-level `apm-injector` command.
package command

import (
	"github.com/spf13/cobra"
)

// MakeCommand creates the top-level `apm-injector` command.
func MakeCommand(subcommands []*cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apm-injector [command]",
		Short: "Datadog APM Injector",
		Long:  `Datadog APM Injector is a tool to install and manage APM instrumentation on hosts and Docker containers.`,
	}

	cmd.AddCommand(subcommands...)

	return cmd
}
