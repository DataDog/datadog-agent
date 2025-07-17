// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kernelbugs

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"syscall"
	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	manager "github.com/DataDog/ebpf-manager"
)

//go:embed c/uprobe-trigger.o
var SimpleUretprobe []byte

//go:embed c/detect-seccomp-bug
var TriggerProgram []byte

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

func HasUretprobeSyscallSeccompBug() (bool, error) {
	const uretprobeSyscallSymbol = "__x64_sys_uretprobe"
	missingSymbols, err := ddebpf.VerifyKernelFuncs(uretprobeSyscallSymbol)
	if err != nil {
		return false, err
	}

	_, isMissing := missingSymbols[uretprobeSyscallSymbol]
	if isMissing {
		return false, nil
	}

	pfile, err := runtime.NewProtectedFile("detect-seccomp-bug", "/tmp", bytes.NewReader(TriggerProgram))
	if err != nil {
		return false, err
	}

	m := manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "uretprobe__segfault",
				},
				BinaryPath:    pfile.Name(),
				MatchFuncName: "trigger_uretprobe_syscall",
			},
		},
	}

	if err := m.Init(bytes.NewReader(SimpleUretprobe)); err != nil {
		return false, err
	}
	defer func() {
		if err := m.Stop(manager.CleanAll); err != nil {
			log.Print(err)
		}
	}()

	if err := m.Start(); err != nil {
		return false, err
	}

	// wait for uprobe to be attached
	time.Sleep(3 * time.Second)

	cmd := exec.Command(pfile.Name())
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			exitcode := exiterr.ExitCode()
			if exitcode == int(syscall.SIGSEGV) {
				return true, nil
			} else {
				return false, fmt.Errorf("unexpected error code %d when probing for uretprobe seccomp bug: %w", exitcode, err)
			}
		} else {
			return false, fmt.Errorf("failed to probe for uretprobe seccomp bug: %w", err)
		}
	}

	return false, nil
}
