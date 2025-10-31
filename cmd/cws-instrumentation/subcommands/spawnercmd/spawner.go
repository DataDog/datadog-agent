// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package spawnercmd holds the spawner command of CWS injector
package spawnercmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// Command returns the commands for the spawner subcommand
func Command() []*cobra.Command {
	healthCmd := &cobra.Command{
		Use:                "spawn",
		Short:              "Spawns a new process, closing all file descriptors except stdin, stdout, stderr before executing the target binary",
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command provided to spawn")
			}

			// by default, all fds opened by the go runtime are CLOEXEC
			// so we only need to take care of the ones we inherit from the parent process

			fdEntries, err := os.ReadDir("/proc/self/fd")
			if err != nil {
				return fmt.Errorf("could not read /proc/self/fd: %w", err)
			}

			for _, entry := range fdEntries {
				name := entry.Name()
				fd, err := strconv.ParseUint(name, 10, 64)
				if err != nil {
					return fmt.Errorf("could not parse fd %s: %w", name, err)
				}

				if fd > 2 {
					if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
						if errors.Is(err, unix.EBADFD) {
							return fmt.Errorf("could not set CLOEXEC on fd %d: %w", fd, err)
						}
					}
				}
			}

			binary := args[0]

			argv0, err := exec.LookPath(binary)
			if err != nil {
				return fmt.Errorf("could not find binary %s in PATH: %w", binary, err)
			}

			return syscall.Exec(argv0, args, os.Environ())

		},
	}

	return []*cobra.Command{healthCmd}
}
