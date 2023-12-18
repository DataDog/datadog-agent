// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Main package for the agent binary
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
	"github.com/spf13/cobra"
)

func coreAgentMain() *cobra.Command {
	common.EnableLoggingToFile()
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 && servicemain.RunningAsWindowsService() {
		servicemain.Run(service.NewWindowsService())
		return nil
	}
	defer log.Flush()

	return command.MakeCommand(subcommands.AgentSubcommands())
}