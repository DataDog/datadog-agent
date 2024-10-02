// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build bootstraper

// Package main implements 'installer'.
package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/installer/subcommands/installer"
	"github.com/DataDog/datadog-agent/cmd/installer/user"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
)

func main() {
	if !user.IsRoot() {
		fmt.Fprintln(os.Stderr, "This command requires root privileges.")
		os.Exit(1)
	}
	cmd := installer.BootstrapCommand()
	cmd.SilenceUsage = true
	os.Exit(runcmd.Run(cmd))
}
