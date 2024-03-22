// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements 'updater'.
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/cmd/updater/subcommands"
)

func main() {
	// root user is changed to dd-updater to avoid permission issues
	rootToDDUpdater()
	os.Exit(runcmd.Run(command.MakeCommand(subcommands.UpdaterSubcommands())))
}
