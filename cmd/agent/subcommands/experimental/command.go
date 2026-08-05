// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package experimental implements undocumented experimental agent subcommands.
package experimental

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	configcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"
	expcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/experimental"
)

// Commands returns the experimental subcommand.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := expcmd.MakeCommand(func() configcmd.GlobalParams {
		return configcmd.GlobalParams{
			ConfFilePath:       globalParams.ConfFilePath,
			ExtraConfFilePaths: globalParams.ExtraConfFilePath,
			ConfigName:         command.ConfigName,
			LoggerName:         command.LoggerName,
		}
	})
	return []*cobra.Command{cmd}
}
