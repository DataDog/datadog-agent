// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package healthcmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

// Command returns the commands for the setup subcommand
func Command() []*cobra.Command {
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Prints OK to stdout",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Print(HealthCommandOutput)
			return nil
		},
	}

	return []*cobra.Command{healthCmd}
}
