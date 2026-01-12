// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kernelbugs provides runtime detection for kernel bugs effecting system-probe
package kernelbugs

import (
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// HasTasksRCUExitLockSymbol returns true if the tasks_rcu_exit_srcu symbol is found in the kernel symbols.
// The tasks_rcu_exit_srcu lock might cause a deadlock when removing fentry trampolines.
// This was fixed by https://github.com/torvalds/linux/commit/1612160b91272f5b1596f499584d6064bf5be794
func HasTasksRCUExitLockSymbol() (bool, error) {
	const tasksRCUExitLockSymbol = "tasks_rcu_exit_srcu"
	missingSymbols, err := ddebpf.VerifyKernelFuncs(tasksRCUExitLockSymbol)
	if err != nil {
		return false, err
	}

	// VerifyKernelFuncs returns the missing symbols
	_, isMissing := missingSymbols[tasksRCUExitLockSymbol]
	return !isMissing, nil
}
