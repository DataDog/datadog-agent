// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package runtime

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	sysprobecmd "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime/policy"
)

func deprecateCommand(cmd *cobra.Command, msg string) *cobra.Command {
	var deprecatedCommand cobra.Command = *cmd
	deprecatedCommand.Deprecated = msg
	return &deprecatedCommand
}

// checkPoliciesCommands is deprecated
func checkPoliciesCommands(globalParams *command.GlobalParams) *cobra.Command {
	confFilePath := ""
	if len(globalParams.ConfigFilePaths) > 0 {
		confFilePath = globalParams.ConfigFilePaths[0]
	}

	sysprobeGlobalParams := sysprobecmd.GlobalParams{
		ConfFilePath:         confFilePath,
		FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
	}

	checkPoliciesCmd := policy.CheckPoliciesCommand(&sysprobeGlobalParams)
	checkPoliciesCmd.Use = "check-policies"
	checkPoliciesCmd.Short = "check policies and return a report"
	return checkPoliciesCmd
}

// reloadPoliciesCommands is deprecated
func reloadPoliciesCommands(globalParams *command.GlobalParams) *cobra.Command {
	confFilePath := ""
	if len(globalParams.ConfigFilePaths) > 0 {
		confFilePath = globalParams.ConfigFilePaths[0]
	}

	sysprobeGlobalParams := sysprobecmd.GlobalParams{
		ConfFilePath:         confFilePath,
		FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
	}

	reloadPoliciesCmd := policy.ReloadPoliciesCommand(&sysprobeGlobalParams)
	reloadPoliciesCmd.Use = "reload"
	reloadPoliciesCmd.Short = "Reload policies"
	return reloadPoliciesCmd
}
