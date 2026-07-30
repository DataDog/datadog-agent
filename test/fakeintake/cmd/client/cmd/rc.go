// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/test/fakeintake/client"
)

// NewRCCommand returns the parent `rc` command for Remote Config control.
func NewRCCommand(cl **client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rc",
		Short: "Manage fakeintake Remote Config state",
		Long: `Push, list, delete and inspect Remote Config payloads served by fakeintake.

Requires the fakeintake server to be started with --remoteconfig.`,
	}
	cmd.AddCommand(
		NewRCAddCommand(cl),
		NewRCListCommand(cl),
		NewRCDeleteCommand(cl),
		NewRCStatsCommand(cl),
		NewRCWatchCommand(cl),
	)
	return cmd
}
