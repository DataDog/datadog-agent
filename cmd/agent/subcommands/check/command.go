// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check implements 'agent check'.
package check

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/cli/subcommands/check"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := check.MakeCommand(func() check.GlobalParams {
		return check.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
			ConfigName:           command.ConfigName,
			LoggerName:           command.LoggerName,
		}
	})

	return []*cobra.Command{cmd}
}
