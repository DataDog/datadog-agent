// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is the main package of CWS injector
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/command"
	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands"
)

func main() {
	rootCmd := command.MakeCommand(subcommands.CWSInjectorSubcommands)

	if err := rootCmd.Execute(); err != nil {
		// the error has already been printed
		os.Exit(-1)
	}
}
