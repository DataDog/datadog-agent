// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tracecmd holds the start command of CWS injector
package tracecmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/selftestscmd"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
)

// EnvDisableStats defines the environ variable to set to disable aviodable stats
const EnvDisableStats = "DD_CWS_INSTRUMENTATION_DISABLE_STATS"

const (
	// gRPCAddr defines the system-probe addr
	probeAddr = "probe-addr"
	// logLevel defines the log level
	verbose = "verbose"
	// uid used to start the tracee
	uid = "uid"
	// gid used to start the tracee
	gid = "gid"
	// async enable the traced program to start and run until we manage to connect to the GRPC endpoint
	async = "async"
	// disableStats -if set- disable the avoidable use of stats to fill more files properties
	disableStats = "disable-stats"
)

type traceCliParams struct {
	ProbeAddr    string
	Verbose      bool
	UID          int32
	GID          int32
	Async        bool
	DisableStats bool
}

// Command returns the commands for the trace subcommand
func Command() []*cobra.Command {
	var params traceCliParams

	traceCmd := &cobra.Command{
		Use:   "trace",
		Short: "trace the syscalls and signals of the given binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds := ptracer.Creds{}
			if params.UID != -1 {
				uid := uint32(params.UID)
				creds.UID = &uid
			}
			if params.GID != -1 {
				gid := uint32(params.GID)
				creds.GID = &gid
			}
			return ptracer.StartCWSPtracer(args, os.Environ(), params.ProbeAddr, creds, params.Verbose, params.Async, params.DisableStats)
		},
	}

	traceCmd.Flags().StringVar(&params.ProbeAddr, probeAddr, setup.DefaultEBPFLessProbeAddr, "system-probe eBPF less GRPC address")
	traceCmd.Flags().BoolVar(&params.Verbose, verbose, false, "enable verbose output")
	traceCmd.Flags().Int32Var(&params.UID, uid, -1, "uid used to start the tracee")
	traceCmd.Flags().Int32Var(&params.GID, gid, -1, "gid used to start the tracee")
	traceCmd.Flags().BoolVar(&params.Async, async, false, "enable async GRPC connection")
	if os.Getenv(EnvDisableStats) != "" {
		params.DisableStats = true
	} else {
		traceCmd.Flags().BoolVar(&params.DisableStats, disableStats, false, "disable use of stats")
	}

	traceCmd.AddCommand(selftestscmd.Command()...)

	return []*cobra.Command{traceCmd}
}
