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
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	processCommand.OneShotLogParams = log.ForOneShot(string(command.LoggerName), "info", true)
	checkAllowlist := []string{"process", "rtprocess", "container", "rtcontainer", "process_discovery"}
	cmd := check.MakeCommand(func() *processCommand.GlobalParams {
		return &processCommand.GlobalParams{
			ConfFilePath:         globalParams.ConfFilePath,
			ExtraConfFilePath:    globalParams.ExtraConfFilePath,
			SysProbeConfFilePath: globalParams.SysProbeConfFilePath,
			FleetPoliciesDirPath: globalParams.FleetPoliciesDirPath,
		}
	},
		"processchecks",
		checkAllowlist,
	)
	return []*cobra.Command{cmd}
}
