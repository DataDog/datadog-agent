// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package setup implements 'agent db setup'.
package setup

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	pgcmd "github.com/DataDog/datadog-agent/cmd/agent/subcommands/db/setup/postgres"
)

// Commands returns the 'db setup' subcommand and its children.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure a database instance for DBM",
		Long:  `Configure a database instance for Datadog Database Monitoring.`,
	}

	for _, sub := range pgcmd.Commands() {
		setupCmd.AddCommand(sub)
	}

	return []*cobra.Command{setupCmd}
}
