// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package preflightmodeimpl

import "syscall"

// preflightProcAttr returns no special attributes: Pdeathsig is Linux-only, so on macOS the
// Go-side stop paths are the only thing preventing an orphaned process.
func preflightProcAttr() *syscall.SysProcAttr { return nil }
