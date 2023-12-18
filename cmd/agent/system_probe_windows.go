// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build bundle_system_probe

// Main package for the agent binary
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/security-agent/windows/service"
	sysprobecommand "github.com/DataDog/datadog-agent/cmd/system-probe/command"
	sysprobesubcommands "github.com/DataDog/datadog-agent/cmd/system-probe/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"

	"github.com/spf13/cobra"
)

func init() {
	registerAgent([]string{"system-probe"}, func() *cobra.Command {
		// if command line arguments are supplied, even in a non-interactive session,
		// then just execute that.  Used when the service is executing the executable,
		// for instance to trigger a restart.
		if len(os.Args) == 1 {
			if servicemain.RunningAsWindowsService() {
				servicemain.Run(&service.Service{})
				return nil
			}
		}
		defer log.Flush()

		rootCmd := sysprobecommand.MakeCommand(sysprobesubcommands.SysprobeSubcommands())
		sysprobecommand.SetDefaultCommandIfNonePresent(rootCmd)
		return rootCmd
	})
}
