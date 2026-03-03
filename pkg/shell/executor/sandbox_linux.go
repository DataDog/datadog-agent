// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package executor

import (
	"os"
	"syscall"
)

// sandboxSysProcAttr returns Linux-specific SysProcAttr that isolates the
// subprocess using user/PID/network/mount/UTS/IPC namespaces. The child is
// killed if the parent dies and gets its own process group.
func sandboxSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getuid(), Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: os.Getgid(), Size: 1},
		},
		Pdeathsig: syscall.SIGKILL,
		Setpgid:   true,
	}
}
