// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processchecks implements 'agent processchecks'.
package processchecks

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	processCommand "github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/cmd/process-agent/subcommands/check"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	processCommand.OneShotLogParams = logimpl.ForOneShot(string(command.LoggerName), "info", true)
	cmd := check.MakeCommand(
		&processCommand.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
		},
		"processchecks",
	)
	return []*cobra.Command{cmd}
}
