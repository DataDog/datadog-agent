// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// host-profiler is a standalone binary that runs the Host Profiler.
package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/cmd/host-profiler/command"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	_ "github.com/DataDog/datadog-agent/pkg/version"
)

// capsForAmbient lists the capabilities the profiler needs in its effective set. They must match
// securityContext.capabilities.add and the file caps set on the binary.
var capsForAmbient = []uintptr{
	unix.CAP_BPF,
	unix.CAP_PERFMON,
	unix.CAP_SYS_PTRACE,
	unix.CAP_SYS_RESOURCE,
	unix.CAP_DAC_READ_SEARCH,
	unix.CAP_SYSLOG,
	unix.CAP_CHECKPOINT_RESTORE,
	unix.CAP_IPC_LOCK,
}


func main() {
	// When running as non-root, file capabilities populate the permitted set at exec time but not
	// the ambient set. Raise each cap to ambient so that it lands in the effective set for this
	// process. Must happen before PR_SET_NO_NEW_PRIVS locks ambient raising.
	for _, cap := range capsForAmbient {
		if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_RAISE, cap, 0, 0); err != nil {
			// Running as root or cap not in permitted set -- not fatal, skip silently.
			break
		}
	}

	// Prevent this process and all children from gaining new privileges via
	// setuid binaries or file capabilities. Inherited across fork/exec.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set PR_SET_NO_NEW_PRIVS: %v\n", err)
		os.Exit(1)
	}

	// Drop from ambient all caps except CAP_SYS_PTRACE, which child processes (e.g. objcopy)
	// need to access target binaries via /proc/<pid>/fd/<n>.
	for _, cap := range capsForAmbient {
		if cap == unix.CAP_SYS_PTRACE {
			continue
		}
		if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_LOWER, cap, 0, 0); err != nil {
			fmt.Fprintf(os.Stderr, "failed to lower ambient capability %d: %v\n", cap, err)
			os.Exit(1)
		}
	}

	flavor.SetFlavor(flavor.HostProfiler)
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
