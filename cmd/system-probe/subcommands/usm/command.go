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

	// Add sysinfo command if available on this platform
	if sysinfoCmd := makeSysinfoCommand(globalParams); sysinfoCmd != nil {
		usmCmd.AddCommand(sysinfoCmd)
	}

	// Add netstat command if available on this platform
	if netstatCmd := makeNetstatCommand(globalParams); netstatCmd != nil {
		usmCmd.AddCommand(netstatCmd)
	}

	// Add check-maps command if available on this platform
	if checkMapsCmd := makeCheckMapsCommand(globalParams); checkMapsCmd != nil {
		usmCmd.AddCommand(checkMapsCmd)
	}

	// Add symbols command if available on this platform
	if symbolsLsCmd := makeSymbolsLsCommand(globalParams); symbolsLsCmd != nil {
		symbolsCmd := &cobra.Command{
			Use:          "symbols",
			Short:        "Symbol inspection commands",
			SilenceUsage: true,
		}
		symbolsCmd.AddCommand(symbolsLsCmd)
		usmCmd.AddCommand(symbolsCmd)
	}

	return []*cobra.Command{usmCmd}
}
