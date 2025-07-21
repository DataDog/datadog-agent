// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && amd64

package kernelbugs

import (
	"bytes"
	// embed is used below to ship the detection programs
	_ "embed"
	"fmt"
	"log"
	"os/exec"
	"syscall"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	manager "github.com/DataDog/ebpf-manager"
)

// SimpleUretprobe holds the bpf bytecode for a uretprobe for detecting a bug in the kernel
//
//go:embed c/uprobe-trigger.o
var SimpleUretprobe []byte

// TriggerProgram holds the bytecode for a userspace program used for detecting a bug in the kernel
//
//go:embed c/detect-seccomp-bug
var TriggerProgram []byte

// HasUretprobeSyscallSeccompBug returns true if the running kernel blocks the uretprobe syscall in seccomp
// This can cause a probed application running within a seccomp context to segfault.
// https://lore.kernel.org/lkml/CAHsH6Gs3Eh8DFU0wq58c_LF8A4_+o6z456J7BidmcVY2AqOnHQ@mail.gmail.com/
var HasUretprobeSyscallSeccompBug = funcs.Memoize(func() (bool, error) {
	return hasUretprobeSyscallSeccompBug()
})

func hasUretprobeSyscallSeccompBug() (bool, error) {
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

	cmd := exec.Command(pfile.Name())
	if err := cmd.Run(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			exitcode := exiterr.ExitCode()
			if exitcode == int(syscall.SIGSEGV) {
				return true, nil
			}

			return false, fmt.Errorf("unexpected error code %d when probing for uretprobe seccomp bug: %w", exitcode, err)
		}

		return false, fmt.Errorf("failed to probe for uretprobe seccomp bug: %w", err)
	}

	return false, nil
}
