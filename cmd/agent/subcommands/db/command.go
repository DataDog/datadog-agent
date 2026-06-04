// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package db implements the 'agent db' subcommand tree used to configure
// databases for Datadog Database Monitoring (DBM).
package db

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/db/setup/postgres"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Configure a database for Datadog Database Monitoring (DBM)",
		Long: `Detect, plan, and apply the database-side setup required for Datadog ` +
			`Database Monitoring.`,
	}

	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure a database for Database Monitoring",
	}
	setupCmd.AddCommand(postgres.Command())

	dbCmd.AddCommand(setupCmd)

	return []*cobra.Command{dbCmd}
}
