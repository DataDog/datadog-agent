// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taggerlist implements 'agent tagger-list'.
package taggerlist

import (
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	taggerlistcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/taggerlist"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := taggerlistcmd.MakeCommand(func() taggerlistcmd.GlobalParams {
		return taggerlistcmd.GlobalParams{
			ConfFilePath: globalParams.ConfFilePath,
			ConfigName:   "datadog",
			LoggerName:   "CORE",
		}
	})

	return []*cobra.Command{cmd}
}
