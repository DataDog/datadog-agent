// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package runtime

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	sysprobecmd "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime/policy"
)

// Commands exports commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "runtime Agent utility commands",
	}

	confFilePath := ""
	if len(globalParams.ConfigFilePaths) != 0 {
		confFilePath = globalParams.ConfigFilePaths[0]
	}

	sysprobeGlobalParams := &sysprobecmd.GlobalParams{
		ConfFilePath:         confFilePath,
		FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
	}

	runtimeCmd.AddCommand(policy.Command(sysprobeGlobalParams))
	runtimeCmd.AddCommand(runtime.SelfTestCommand(sysprobeGlobalParams))
	/*
		runtimeCmd.AddCommand(activityDumpCommands(globalParams)...)
		runtimeCmd.AddCommand(securityProfileCommands(globalParams)...)
		runtimeCmd.AddCommand(processCacheCommands(globalParams)...)
		runtimeCmd.AddCommand(networkNamespaceCommands(globalParams)...)
		runtimeCmd.AddCommand(discardersCommands(globalParams)...)

	*/

	// Deprecated
	runtimeCmd.AddCommand(
		deprecateCommand(checkPoliciesCommands(globalParams), "please use `security-agent runtime policy check` instead"),
		deprecateCommand(reloadPoliciesCommands(globalParams), "please use `security-agent runtime policy reload` instead"))

	return []*cobra.Command{runtimeCmd}
}
