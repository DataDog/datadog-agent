// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/cws-injector/command"
	"github.com/DataDog/datadog-agent/cmd/cws-injector/subcommands"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
)

func main() {
	rootCmd := command.MakeCommand(subcommands.CWSInjectorSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
