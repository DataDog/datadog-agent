// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the main package of CWS profiler
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/profiler/command"
	"github.com/DataDog/datadog-agent/cmd/profiler/subcommands"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
)

func main() {
	rootCmd := command.MakeCommand(subcommands.CWSProfilerSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
