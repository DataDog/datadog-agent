// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tracecmd holds the start command of CWS injector
package tracecmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/selftestscmd"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
)

const (
	// gRPCAddr defines the system-probe addr
	probeAddr = "probe-addr"
	// logLevel defines the log level
	verbose = "verbose"
)

type traceCliParams struct {
	ProbeAddr string
	Verbose   bool
}

// Command returns the commands for the trace subcommand
func Command() []*cobra.Command {
	var params traceCliParams

	traceCmd := &cobra.Command{
		Use:   "trace",
		Short: "trace the syscalls and signals of the given binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ptracer.StartCWSPtracer(args, params.ProbeAddr, params.Verbose)
		},
	}

	traceCmd.Flags().StringVar(&params.ProbeAddr, probeAddr, "localhost:5678", "system-probe eBPF less GRPC address")
	traceCmd.Flags().BoolVar(&params.Verbose, verbose, false, "enable verbose output")

	traceCmd.AddCommand(selftestscmd.Command()...)

	return []*cobra.Command{traceCmd}
}
