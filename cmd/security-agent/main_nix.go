// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/compliance"
	subconfig "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/config"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/flare"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/start"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/status"
	subversion "github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/version"
	"os"

	_ "expvar"         // Blank import used because this isn't directly used in this file
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/DataDog/datadog-agent/pkg/util/flavor"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
)

func main() {
	// set the Agent flavor
	flavor.SetFlavor(flavor.SecurityAgent)

	subcommandFactories := []command.SubcommandFactory{
		status.Commands,
		flare.Commands,
		subconfig.Commands,
		compliance.Commands,
		runtime.Commands,
		subversion.Commands,
		start.Commands,
	}

	if err := command.MakeCommand(subcommandFactories).Execute(); err != nil {
		os.Exit(-1)
	}
}
