// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && bundle_trace_agent

// Main package for the agent binary
package main

import (
	"os"

	tracecommand "github.com/DataDog/datadog-agent/cmd/trace-agent/command"
	"github.com/spf13/cobra"
)

func init() {
	registerAgent("trace-agent", func() *cobra.Command {
		os.Args = tracecommand.FixDeprecatedFlags(os.Args, os.Stdout)
		return tracecommand.MakeRootCommand()
	})
}
