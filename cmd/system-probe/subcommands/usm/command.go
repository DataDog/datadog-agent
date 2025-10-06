// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package usm provides debugging and diagnostic commands for Universal Service Monitoring.
package usm

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

// Commands returns a slice containing the USM top-level command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	usmCmd := &cobra.Command{
		Use:          "usm",
		Short:        "Universal Service Monitoring commands",
		SilenceUsage: true,
	}

	usmCmd.AddCommand(makeConfigCommand(globalParams))
	usmCmd.AddCommand(makeSysinfoCommand(globalParams))

	return []*cobra.Command{usmCmd}
}
