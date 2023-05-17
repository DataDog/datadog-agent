// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/subcommands"
)

func main() {
	rootCmd := command.MakeCommand(subcommands.SysprobeSubcommands())
	setDefaultCommandIfNonePresent(rootCmd)
	os.Exit(runcmd.Run(rootCmd))
}
