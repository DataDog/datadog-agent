// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Main package for the allinone binary
package main

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/spf13/cobra"
)

var agents = map[string]func() *cobra.Command{}

func registerAgent(getCommand func() *cobra.Command, names ...string) {
	for _, name := range names {
		agents[name] = getCommand
	}
}

func main() {
	executable := path.Base(os.Args[0])
	process := strings.TrimSuffix(executable, path.Ext(executable))

	if agentCmdBuilder := agents[process]; agentCmdBuilder != nil {
		rootCmd := agentCmdBuilder()
		os.Exit(runcmd.Run(rootCmd))
	}

	fmt.Fprintf(os.Stderr, "'%s' is an incorrect invocation of the Datadog Agent\n", process)
	os.Exit(1)
}
