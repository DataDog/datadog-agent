// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func main() {
	// Set the flavor
	flavor.SetFlavor(flavor.IotAgent)

	if err := command.MakeCommand(subcommands.AgentSubcommands()).Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
