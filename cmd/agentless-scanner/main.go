// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements main
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/command"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	flavor.SetFlavor(flavor.AgentlessScanner)

	signal.Ignore(syscall.SIGPIPE)

	cmd := command.RootCommand()
	cmd.SilenceErrors = true
	err := cmd.Execute()

	if err != nil {
		log.Flush()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}
	log.Flush()
	os.Exit(0)
}
