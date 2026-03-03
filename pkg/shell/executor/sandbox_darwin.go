// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package executor

import "syscall"

// sandboxSysProcAttr returns macOS-specific SysProcAttr. macOS does not
// support Linux namespaces, so the sandbox is minimal — protection comes
// from the restricted interpreter itself.
func sandboxSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
