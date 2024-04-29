// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tracecmd holds the start command of CWS injector
package tracecmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/selftestscmd"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
)

const (
	// envStatsDisabled defines the environment variable to set to disable avoidable stats
	envStatsDisabled = "DD_CWS_INSTRUMENTATION_STATS_DISABLED"
	// envProcScanDisabled defines the environment variable to disable procfs scan
	envProcScanDisabled = "DD_CWS_INSTRUMENTATION_PROC_SCAN_DISABLED"
	// envProcScanRate defines the rate of the prodfs scan
	envProcScanRate = "DD_CWS_INSTRUMENTATION_PROC_SCAN_RATE"
	// envSeccompDisabled defines the environment variable to disable seccomp
	envSeccompDisabled = "DD_CWS_INSTRUMENTATION_SECCOMP_DISABLED"
)

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
	// disableProcScan disable the procfs scan
	disableProcScan = "disable-proc-scan"
	// scanProcEvery procfs scan rate
	scanProcEvery = "proc-scan-rate"
	// disableSeccomp disable seccomp
	disableSeccomp = "disable-seccomp"
)

type traceCliParams struct {
	ProbeAddr        string
	Verbose          bool
	UID              int32
	GID              int32
	Async            bool
	StatsDisabled    bool
	ProcScanDisabled bool
	ScanProcEvery    string
	SeccompDisabled  bool
}

func envToBool(name string) bool {
	return strings.ToLower(os.Getenv(name)) == "true"
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

			opts := ptracer.Opts{
				Creds:            creds,
				Verbose:          params.Verbose,
				Async:            params.Async,
				StatsDisabled:    params.StatsDisabled,
				ProcScanDisabled: params.ProcScanDisabled,
				SeccompDisabled:  params.SeccompDisabled,
			}

			if params.ScanProcEvery != "" {
				every, err := time.ParseDuration(params.ScanProcEvery)
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid scan proc rate duration `%s`: %s", params.ScanProcEvery, err)
				}
				opts.ScanProcEvery = every
			}

			return ptracer.StartCWSPtracer(args, os.Environ(), params.ProbeAddr, opts)
		},
	}

	traceCmd.Flags().StringVar(&params.ProbeAddr, probeAddr, constants.DefaultEBPFLessProbeAddr, "system-probe eBPF less GRPC address")
	traceCmd.Flags().BoolVar(&params.Verbose, verbose, false, "enable verbose output")
	traceCmd.Flags().Int32Var(&params.UID, uid, -1, "uid used to start the tracee")
	traceCmd.Flags().Int32Var(&params.GID, gid, -1, "gid used to start the tracee")
	traceCmd.Flags().BoolVar(&params.Async, async, false, "enable async GRPC connection")
	traceCmd.Flags().BoolVar(&params.StatsDisabled, disableStats, envToBool(envStatsDisabled), "disable use of stats")
	traceCmd.Flags().BoolVar(&params.ProcScanDisabled, disableProcScan, envToBool(envProcScanDisabled), "disable proc scan")
	traceCmd.Flags().StringVar(&params.ScanProcEvery, scanProcEvery, os.Getenv(envProcScanRate), "proc scan rate")
	traceCmd.Flags().BoolVar(&params.SeccompDisabled, disableSeccomp, envToBool(envSeccompDisabled), "disable seccomp")

	traceCmd.AddCommand(selftestscmd.Command()...)

	return []*cobra.Command{traceCmd}
}
