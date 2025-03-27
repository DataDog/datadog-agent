// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package runtime

import (
	"github.com/spf13/cobra"

	sysprobecmd "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime/policy"
)

func deprecateCommand(cmd *cobra.Command, msg string) *cobra.Command {
	var deprecatedCommand cobra.Command = *cmd
	deprecatedCommand.Deprecated = msg
	return &deprecatedCommand
}

// checkPoliciesCommand is deprecated
func checkPoliciesCommand(globalParams *sysprobecmd.GlobalParams) *cobra.Command {
	checkPoliciesCmd := policy.CheckPoliciesCommand(globalParams)
	checkPoliciesCmd.Use = "check-policies"
	checkPoliciesCmd.Short = "check policies and return a report"
	return checkPoliciesCmd
}

// reloadPoliciesCommand is deprecated
func reloadPoliciesCommand(globalParams *sysprobecmd.GlobalParams) *cobra.Command {

	reloadPoliciesCmd := policy.ReloadPoliciesCommand(globalParams)
	reloadPoliciesCmd.Use = "reload"
	reloadPoliciesCmd.Short = "Reload policies"
	return reloadPoliciesCmd
}
