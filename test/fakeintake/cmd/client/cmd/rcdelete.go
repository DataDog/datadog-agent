// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCDeleteCommand returns the `rc delete` subcommand.
func NewRCDeleteCommand(cl **client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <key>",
		Short: "Delete a Remote Config entry by its <org>/<product>/<config_id>/<config_name> key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return (*cl).RCDeleteConfig(args[0])
		},
	}
	return cmd
}
