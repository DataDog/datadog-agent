// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

// Package main is the entrypoint for the par-executor binary.
// par-executor is the execution plane of the PAR dual-process architecture.
// It is spawned on-demand by the par-control Rust binary.
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/par-executor/command"
	"github.com/DataDog/datadog-agent/cmd/par-executor/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func main() {
	flavor.SetFlavor(flavor.PrivateActionRunner)
	rootCmd := command.MakeCommand(subcommands.ParExecutorSubcommands())
	os.Exit(runcmd.Run(rootCmd))
}
