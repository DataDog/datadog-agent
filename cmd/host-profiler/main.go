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

func main() {
	// Prevent this process and all children from gaining new privileges via
	// setuid binaries or file capabilities. Inherited across fork/exec.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set PR_SET_NO_NEW_PRIVS: %v\n", err)
		os.Exit(1)
	}

	flavor.SetFlavor(flavor.HostProfiler)
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
