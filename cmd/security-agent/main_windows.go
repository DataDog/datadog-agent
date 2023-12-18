// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package main implements main
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands"
	"github.com/DataDog/datadog-agent/cmd/security-agent/windows/service"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

func main() {
	// if command line arguments are supplied, even in a non-interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		if servicemain.RunningAsWindowsService() {
			servicemain.Run(&service.Service{})
			return
		}
	}

	rootCmd := command.MakeCommand(subcommands.SecurityAgentSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
