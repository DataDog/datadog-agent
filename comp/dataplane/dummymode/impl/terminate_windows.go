// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package dummymodeimpl

import (
	"os"
	"syscall"
)

// requestStop terminates ADP. Windows has no signal a console-less child reliably observes, so
// the graceful shutdown path is not exercised here — under normal packaging ADP is stopped by
// dd-procmgr rather than by us.
func requestStop(p *os.Process) error {
	return p.Kill()
}

// dummyProcAttr returns no special attributes. Pdeathsig is Linux-only and Windows has no
// portable equivalent worth the complexity here, so the Go-side stop paths are the only thing
// preventing an orphaned process.
func dummyProcAttr() *syscall.SysProcAttr { return nil }
