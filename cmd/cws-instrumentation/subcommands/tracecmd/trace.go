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
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/selftestscmd"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
)

const (
	// envDisableStats defines the environment variable to set to disable avoidable stats
	envDisableStats = "DD_CWS_INSTRUMENTATION_DISABLE_STATS"
	// envDisableProcScan defines the environment variable to disable procfs scan
	envDisableProcScan = "DD_CWS_INSTRUMENTATION_DISABLE_PROC_SCAN"
	// envProcScanRate defines the rate of the prodfs scan
	envProcScanRate = "DD_CWS_INSTRUMENTATION_PROC_SCAN_RATE"
	// envDisableSeccomp defines the environment variable to disable seccomp
	envDisableSeccomp = "DD_CWS_INSTRUMENTATION_DISABLE_SECCOMP"
)

const (
	// probeAddrOpt defines the system-probe addr
	probeAddrOpt = "probe-addr"
	// verboseOpt defines the log level
	verboseOpt = "verbose"
	// uidOpt used to start the tracee
	uidOpt = "uid"
	// gidOpt used to start the tracee
	gidOpt = "gid"
	// asyncOpt enable the traced program to start and run until we manage to connect to the GRPC endpoint
	asyncOpt = "async"
	// disableStatsOpt -if set- disable the avoidable use of stats to fill more files properties
	disableStatsOpt = "disable-stats"
	// disableProcScanOpt disable the procfs scan
	disableProcScanOpt = "disable-proc-scan"
	// scanProcEveryOpt procfs scan rate
	scanProcEveryOpt = "proc-scan-rate"
	// disableSeccompOpt disable seccomp
	disableSeccompOpt = "disable-seccomp"
	// pidOpt attach mode
	pidOpt = "pid"
)

type traceCliParams struct {
	ProbeAddr       string
	Verbose         bool
	UID             int32
	GID             int32
	Async           bool
	DisableStats    bool
	DisableProcScan bool
	ScanProcEvery   string
	DisableSeccomp  bool
	PID             int
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
				Creds:           creds,
				Verbose:         params.Verbose,
				Async:           params.Async,
				DisableStats:    params.DisableStats,
				DisableProcScan: params.DisableProcScan,
				DisableSeccomp:  params.DisableSeccomp,
			}

			if params.ScanProcEvery != "" {
				every, err := time.ParseDuration(params.ScanProcEvery)
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid scan proc rate duration `%s`: %s", params.ScanProcEvery, err)
				}
				opts.ScanProcEvery = every
			}

			if params.PID > 0 {
				return ptracer.Attach(params.PID, params.ProbeAddr, opts)
			}
			return ptracer.Wrap(args, os.Environ(), params.ProbeAddr, opts)
		},
	}

	traceCmd.Flags().StringVar(&params.ProbeAddr, probeAddrOpt, constants.DefaultEBPFLessProbeAddr, "system-probe eBPF less GRPC address")
	traceCmd.Flags().BoolVar(&params.Verbose, verboseOpt, false, "enable verbose output")
	traceCmd.Flags().Int32Var(&params.UID, uidOpt, -1, "uid used to start the tracee")
	traceCmd.Flags().Int32Var(&params.GID, gidOpt, -1, "gid used to start the tracee")
	traceCmd.Flags().BoolVar(&params.Async, asyncOpt, false, "enable async GRPC connection")
	traceCmd.Flags().BoolVar(&params.DisableStats, disableStatsOpt, os.Getenv(envDisableStats) != "", "disable use of stats")
	traceCmd.Flags().BoolVar(&params.DisableProcScan, disableProcScanOpt, os.Getenv(envDisableProcScan) != "", "disable proc scan")
	traceCmd.Flags().StringVar(&params.ScanProcEvery, scanProcEveryOpt, os.Getenv(envProcScanRate), "proc scan rate")
	traceCmd.Flags().BoolVar(&params.DisableSeccomp, disableSeccompOpt, os.Getenv(envDisableSeccomp) != "", "disable seccomp")
	traceCmd.Flags().IntVar(&params.PID, pidOpt, -1, "attach tracer to pid")

	traceCmd.AddCommand(selftestscmd.Command()...)

	return []*cobra.Command{traceCmd}
}
