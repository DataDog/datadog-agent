// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands/run"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/servicemain"
)

func main() {
	flavor.SetFlavor(flavor.PrivateActionRunner)

	// SCM gives services 30 seconds to call StartServiceCtrlDispatcher
	if len(os.Args) == 1 && servicemain.RunningAsWindowsService() {
		servicemain.Run(run.NewService())
		return
	}

	rootCmd := command.MakeCommand(subcommands.PrivateActionRunnerSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
