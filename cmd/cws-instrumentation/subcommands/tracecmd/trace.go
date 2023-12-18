// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package tracecmd holds the start command of CWS injector
package tracecmd

import (
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/security/ptracer"
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
)

type traceCliParams struct {
	ProbeAddr string
	Verbose   bool
	UID       int32
	GID       int32
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
			return ptracer.StartCWSPtracer(args, params.ProbeAddr, creds, params.Verbose)
		},
	}

	traceCmd.Flags().StringVar(&params.ProbeAddr, probeAddr, "localhost:5678", "system-probe eBPF less GRPC address")
	traceCmd.Flags().BoolVar(&params.Verbose, verbose, false, "enable verbose output")
	traceCmd.Flags().Int32Var(&params.UID, uid, -1, "uid used to start the tracee")
	traceCmd.Flags().Int32Var(&params.GID, gid, -1, "gid used to start the tracee")

	return []*cobra.Command{traceCmd}
}
