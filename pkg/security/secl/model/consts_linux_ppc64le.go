// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package model

import (
        "golang.org/x/sys/unix"
)

var (
        ptraceArchConstants = map[string]uint32{
                "PTRACE_GETEVRREGS":        unix.PTRACE_GETEVRREGS,
                "PTRACE_GETFPREGS":         unix.PTRACE_GETFPREGS,
                "PTRACE_GETREGS64":         unix.PTRACE_GETREGS64,
                "PTRACE_GETVRREGS":         unix.PTRACE_GETVRREGS,
                "PTRACE_GETVSRREGS":        unix.PTRACE_GETVSRREGS,
                "PTRACE_GET_DEBUGREG":      unix.PTRACE_GET_DEBUGREG,
                "PTRACE_SETEVRREGS":        unix.PTRACE_SETEVRREGS,
                "PTRACE_SETFPREGS":         unix.PTRACE_SETFPREGS,
                "PTRACE_SETREGS64":         unix.PTRACE_SETREGS64,
                "PTRACE_SETVRREGS":         unix.PTRACE_SETVRREGS,
                "PTRACE_SETVSRREGS":        unix.PTRACE_SETVSRREGS,
                "PTRACE_SET_DEBUGREG":      unix.PTRACE_SET_DEBUGREG,
                "PTRACE_SINGLEBLOCK":       unix.PTRACE_SINGLEBLOCK,
                "PTRACE_SYSEMU":                    unix.PTRACE_SYSEMU,
                "PTRACE_SYSEMU_SINGLESTEP":    unix.PTRACE_SYSEMU_SINGLESTEP,
        }

        mmapFlagArchConstants = map[string]int{}
)

