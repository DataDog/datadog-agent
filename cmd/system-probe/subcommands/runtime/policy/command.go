// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package policy holds policy CLI subcommand related files
package policy

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"

	"github.com/spf13/cobra"
)

// Command returns the CLI command for "runtime policy"
func Command(globalParams *command.GlobalParams) *cobra.Command {
	policyCmd := &cobra.Command{
		Use:   "policy",
		Short: "Policy related commands",
	}

	policyCmd.AddCommand(
		EvalCommand(globalParams),
		CheckPoliciesCommand(globalParams),
		ReloadPoliciesCommand(globalParams),
		DownloadPolicyCommand(globalParams),
	)

	return policyCmd
}
