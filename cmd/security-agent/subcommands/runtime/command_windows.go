// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package runtime

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
)

// Commands exports commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	runtimeCmd.AddCommand(commonPolicyCommands(globalParams)...)
	/*
		runtimeCmd.AddCommand(selfTestCommands(globalParams)...)
		runtimeCmd.AddCommand(activityDumpCommands(globalParams)...)
		runtimeCmd.AddCommand(securityProfileCommands(globalParams)...)
		runtimeCmd.AddCommand(processCacheCommands(globalParams)...)
		runtimeCmd.AddCommand(networkNamespaceCommands(globalParams)...)
		runtimeCmd.AddCommand(discardersCommands(globalParams)...)

	*/
	// Deprecated
	runtimeCmd.AddCommand(checkPoliciesCommands(globalParams)...)
	runtimeCmd.AddCommand(reloadPoliciesCommands(globalParams)...)

	return []*cobra.Command{runtimeCmd}
}
