// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package features contains feature detection for eBPF
package features

import (
	"errors"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/ebpf/kernelbugs"
)

// SupportsFexit returns nil if the kernel supports fexit functions and has no known related bugs.
func SupportsFexit(funcName string) error {
	return supportFentryFexit(funcName, ebpf.AttachTraceFExit)
}

// SupportsFentry returns nil if the kernel supports fentry/fexit functions and has no known related bugs.
func SupportsFentry(funcName string) error {
	return supportFentryFexit(funcName, ebpf.AttachTraceFEntry)
}

func supportFentryFexit(funcName string, attachType ebpf.AttachType) error {
	if err := features.HaveProgramType(ebpf.Tracing); err != nil {
		return err
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: attachType,
		AttachTo:   funcName,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
	}
	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return err
	}
	defer prog.Close()

	// deadlock check must come before attach, since deadlock is upon detach
	hasPotentialFentryDeadlock, err := kernelbugs.HasTasksRCUExitLockSymbol()
	if err != nil {
		// in case of error, let's be safe and assume the bug is present
		return err
	} else if hasPotentialFentryDeadlock {
		return errors.New("has potential fentry deadlock")
	}

	l, err := link.AttachTracing(link.TracingOptions{
		Program: prog,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	return nil
}
