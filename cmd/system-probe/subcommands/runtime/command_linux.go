// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands/runtime/policy"
)

// Commands returns the config commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "Runtime Security Agent (CWS) utility commands",
	}

	runtimeCmd.AddCommand(policy.Command(globalParams))
	runtimeCmd.AddCommand(SelfTestCommand(globalParams))
	runtimeCmd.AddCommand(ActivityDumpCommand(globalParams))
	runtimeCmd.AddCommand(SecurityProfileCommand(globalParams))
	runtimeCmd.AddCommand(ProcessCacheCommand(globalParams))
	runtimeCmd.AddCommand(NetworkNamespaceCommand(globalParams))
	runtimeCmd.AddCommand(DiscardersCommand(globalParams))

	return []*cobra.Command{runtimeCmd}
}
