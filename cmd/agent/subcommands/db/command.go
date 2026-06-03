// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package db implements 'agent db'.
package db

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	cmdsetup "github.com/DataDog/datadog-agent/cmd/agent/subcommands/db/setup"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Database monitoring tools",
		Long:  `Commands for configuring and managing Datadog Database Monitoring.`,
	}

	for _, sub := range cmdsetup.Commands(globalParams) {
		dbCmd.AddCommand(sub)
	}

	return []*cobra.Command{dbCmd}
}
